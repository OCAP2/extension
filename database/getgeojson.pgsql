SELECT jsonb_build_object(
    'type',     'FeatureCollection',
    'features', jsonb_agg(features.feature)
)
FROM (
  SELECT jsonb_build_object(
    'type',       'Feature',
    'id',         inputs.id,
    'geometry',   ST_AsGeoJSON(position)::jsonb,
    'properties', to_jsonb(inputs) - 'position' - 'id'
  ) AS feature
  FROM (
    SELECT
    -- concat ocap_id and capture_frame to create a unique id
    concat(soldier_states.capture_frame, '_', soldiers.ocap_id) AS id,
    soldiers.ocap_id,
    soldiers.group_id,
    soldiers.side,
    soldiers.role_description,
    soldiers.player_uid,
    soldier_states."time",
    soldier_states.capture_frame AS capture_frame,
    -- position as 4326 jsonb geojson
    soldier_states.position AS position,
    soldier_states.bearing AS bearing,
    soldier_states.lifestate,
    soldier_states.unit_name as "name",
    soldier_states.is_player as is_player,
    soldier_states.has_stable_vitals,
    soldier_states.is_dragged_carried
FROM soldiers
    JOIN soldier_states ON soldier_states.soldier_id = soldiers.id
WHERE
    capture_frame BETWEEN 0 AND 100
    AND mission_id in (?)
ORDER BY capture_frame ASC
  ) inputs) features;