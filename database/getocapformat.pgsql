-- Get data in OCAP format from the database
-- @param db Database connection
WITH states AS (
  SELECT
    jsonb_build_array(jsonb_build_array(ST_X(position), ST_Y(position), elevation_asl), bearing, lifestate, in_vehicle::int, unit_name, is_player::int, "current_role") AS state,
    soldier_id
  FROM
    soldier_states
  ORDER BY
    capture_frame ASC
),
soldiers AS (
  SELECT
    jsonb_build_object('joinTime', TO_CHAR(join_time, 'YYYY/MM/DD HH:MM:SS'), 'id', ocap_id, 'group', group_id, 'side', side, 'isPlayer', is_player::int, 'role', role_description, 'playerUid', player_uid, 'startFrameNum', join_frame,
      -- use custom value of "type"
      'type', 'unit', 'positions', json_agg(ss.state)) AS soldier
  FROM
    soldiers s
    LEFT JOIN states ss ON ss.soldier_id = s.id
  WHERE
    s.mission_id IN ($1)
  GROUP BY
    s.id,
    s.join_time,
    s.ocap_id,
    s.group_id,
    s.side,
    s.is_player,
    s.role_description,
    s.player_uid,
    s.join_frame
)
SELECT
  jsonb_build_object('entities', jsonb_agg(soldiers.soldier))
FROM
  soldiers
LIMIT 500;

