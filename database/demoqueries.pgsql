-- get all data for a frame

-- first we need to specify the mission (id 2)

SELECT * FROM missions WHERE id = 2;

-- then get all soldiers and vehicles

SELECT * FROM soldiers WHERE mission_id = 2;

SELECT * FROM vehicles WHERE mission_id = 2;

-- save their ids in a list

SELECT id FROM soldiers WHERE mission_id = 2;

SELECT id FROM vehicles WHERE mission_id = 2;

-- then get all states where soldier_id is in the list of soldiers and capture frame is between 0 and 100

SELECT
    soldiers.*,
    soldier_states.*
FROM soldiers
    JOIN soldier_states ON soldier_states.soldier_id = soldiers.id
WHERE
    capture_frame BETWEEN 0 AND 100
    AND mission_id = 2
ORDER BY capture_frame ASC;

SELECT
    soldiers.ocap_id,
    soldiers.group_id,
    soldiers.side,
    soldiers.role_description,
    soldiers.player_uid,
    soldier_states."time",
    soldier_states.capture_frame AS capture_frame,
    ST_X (
        ST_TRANSFORM(soldier_states.position, 4326)
    ) AS lon,
    ST_Y(
        ST_TRANSFORM(soldier_states.position, 4326)
    ) AS lat,
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
    AND mission_id in ($1)
ORDER BY capture_frame ASC;

-- GEOJSON


