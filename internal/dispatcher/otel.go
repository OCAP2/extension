package dispatcher

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

const instrumentationName = "github.com/OCAP2/extension/v5/internal/dispatcher"

func meter() metric.Meter {
	return otel.Meter(instrumentationName)
}
