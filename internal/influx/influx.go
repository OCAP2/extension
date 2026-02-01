package influx

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2_api "github.com/influxdata/influxdb-client-go/v2/api"
	influxdb2_write "github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/influxdata/influxdb-client-go/v2/domain"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

// DefaultBucketNames are the default InfluxDB buckets used by OCAP.
var DefaultBucketNames = []string{
	"mission_data",
	"ocap_performance",
	"player_performance",
	"server_performance",
	"Telegraf",
}

// Manager handles InfluxDB connections and writes.
type Manager struct {
	Client       influxdb2.Client
	Writers      map[string]influxdb2_api.WriteAPI
	BackupWriter *gzip.Writer
	IsValid      bool
	BucketNames  []string
	Logger       zerolog.Logger
	BackupPath   string
}

// NewManager creates a new InfluxDB manager.
func NewManager(log zerolog.Logger, backupPath string) *Manager {
	return &Manager{
		Writers:     make(map[string]influxdb2_api.WriteAPI),
		IsValid:     false,
		BucketNames: DefaultBucketNames,
		Logger:      log,
		BackupPath:  backupPath,
	}
}

// Connect establishes a connection to InfluxDB.
func (m *Manager) Connect() error {
	if !viper.GetBool("influx.enabled") {
		return errors.New("influxdb.Enabled is false")
	}

	m.Client = influxdb2.NewClientWithOptions(
		fmt.Sprintf(
			"%s://%s:%s",
			viper.GetString("influx.protocol"),
			viper.GetString("influx.host"),
			viper.GetString("influx.port"),
		),
		viper.GetString("influx.token"),
		influxdb2.DefaultOptions().
			SetBatchSize(2500).
			SetFlushInterval(1000),
	)

	// validate client connection health
	running, err := m.Client.Ping(context.Background())

	if err != nil || !running {
		m.IsValid = false
		// create backup writer
		if m.BackupWriter == nil {
			m.Logger.Info().Str("backupPath", m.BackupPath).
				Msg("Failed to initialize InfluxDB client, writing to backup file")

			file, err := os.OpenFile(m.BackupPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("error creating backup file: %v", err)
			}
			m.BackupWriter = gzip.NewWriter(file)
		}
	} else {
		m.IsValid = true
	}

	if m.IsValid {
		err = m.setupOrganizationAndBuckets()
		if err != nil {
			return err
		}
		m.CreateWriters()
		m.Logger.Info().Msg("InfluxDB client initialized")
	} else {
		m.Logger.Warn().Msg("InfluxDB client failed to initialize, using backup writer")
	}

	return nil
}

func (m *Manager) setupOrganizationAndBuckets() error {
	ctx := context.Background()
	orgName := viper.GetString("influx.org")

	// ensure org exists
	_, err := m.Client.OrganizationsAPI().FindOrganizationByName(ctx, orgName)
	if err != nil {
		m.Logger.Info().Str("org", orgName).Msg("Organization not found, creating")
		_, err = m.Client.OrganizationsAPI().CreateOrganizationWithName(ctx, orgName)
		if err != nil {
			m.Logger.Error().Err(err).Str("org", orgName).Msg("Error creating organization")
			return err
		}
	}

	// get influxOrg
	influxOrg, err := m.Client.OrganizationsAPI().FindOrganizationByName(ctx, orgName)
	if err != nil {
		m.Logger.Error().Err(err).Str("org", orgName).Msg("Error getting organization")
		return err
	}

	// ensure buckets exist with 90 day retention
	for _, bucket := range m.BucketNames {
		_, err = m.Client.BucketsAPI().FindBucketByName(ctx, bucket)
		if err != nil {
			m.Logger.Info().Str("bucket", bucket).Msg("Bucket not found, creating")

			rule := domain.RetentionRuleTypeExpire
			_, err = m.Client.BucketsAPI().CreateBucketWithName(ctx, influxOrg, bucket, domain.RetentionRule{
				Type:         &rule,
				EverySeconds: 60 * 60 * 24 * 90, // 90 days
			})
			if err != nil {
				m.Logger.Error().Err(err).Str("bucket", bucket).Msg("Error creating bucket")
				return err
			}
		}
	}

	return nil
}

// CreateWriters creates write APIs for all configured buckets.
func (m *Manager) CreateWriters() {
	orgName := viper.GetString("influx.org")
	for _, bucket := range m.BucketNames {
		m.Logger.Trace().Str("bucket", bucket).Msg("Creating InfluxDB writer")
		m.Writers[bucket] = m.Client.WriteAPI(orgName, bucket)

		errorsCh := m.Writers[bucket].Errors()
		go func(bucketName string, errorsCh <-chan error) {
			for writeErr := range errorsCh {
				m.Logger.Error().Err(writeErr).Str("bucket", bucketName).
					Msg("Error sending data to InfluxDB")
			}
		}(bucket, errorsCh)

		m.Logger.Trace().Str("bucket", bucket).Msg("InfluxDB writer created")
	}

	m.Logger.Debug().Msg("InfluxDB writers initialized")
}

// WritePoint writes a point to InfluxDB or backup file.
func (m *Manager) WritePoint(ctx context.Context, bucket string, point *influxdb2_write.Point) error {
	if m.IsValid {
		if _, ok := m.Writers[bucket]; !ok {
			return fmt.Errorf("influxDB bucket '%s' not registered", bucket)
		}
		m.Writers[bucket].WritePoint(point)
	} else {
		if m.BackupWriter == nil {
			return fmt.Errorf("influxDB client not initialized and backup writer not available")
		}

		lineProtocol := influxdb2_write.PointToLineProtocol(point, time.Duration(1*time.Nanosecond))
		_, err := m.BackupWriter.Write([]byte(lineProtocol + "\n"))
		if err != nil {
			return fmt.Errorf("error writing to InfluxDB backup file: %s", err)
		}
	}

	return nil
}

// ProcessMetricData parses metric data from Arma and returns a bucket name and point.
func ProcessMetricData(data []string, fixEscapeQuotes func(string) string, trimQuotes func(string) string) (
	bucket string,
	point *influxdb2_write.Point,
	err error,
) {
	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	// each metric will come through as a string array
	// 0 = bucket name
	// 1 = measurement name
	// n with "tag" prefix = tag name
	// n with "field" prefix = field
	// tag and field values use "::" separator

	bucket = data[0]
	measurementName := data[1]
	point = influxdb2_write.NewPointWithMeasurement(measurementName)

	// add tags
	for _, tag := range data[2:] {
		if !strings.HasPrefix(tag, "tag::") {
			continue
		}
		parts := strings.Split(tag, "::")
		if len(parts) >= 3 {
			point.AddTag(parts[1], parts[2])
		}
	}

	// add fields
	for _, field := range data[2:] {
		if !strings.HasPrefix(field, "field::") {
			continue
		}
		parts := strings.Split(field, "::")
		if len(parts) < 4 {
			continue
		}
		fieldType := parts[1]
		fieldName := parts[2]
		fieldValue := parts[3]

		switch fieldType {
		case "string":
			point.AddField(fieldName, fieldValue)
		case "int":
			intVal, err := strconv.Atoi(fieldValue)
			if err != nil {
				return "", nil, fmt.Errorf("error converting field value '%s' to int: %w", fieldValue, err)
			}
			point.AddField(fieldName, intVal)
		case "float":
			floatVal, err := strconv.ParseFloat(fieldValue, 64)
			if err != nil {
				return "", nil, fmt.Errorf("error converting field value '%s' to float: %w", fieldValue, err)
			}
			point.AddField(fieldName, floatVal)
		}
	}

	return bucket, point, nil
}
