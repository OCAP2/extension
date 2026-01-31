-- addons view
create view addons_to_missions as (
  select m.id as mission_id,
    a.id as addon_id,
    a.name as addon_name,
    a.workshop_id as addon_workshop_id
  from missions m
    left join mission_addons ma on ma.mission_id = m.id
    left join addons a on a.id = ma.addon_id
);
-- main missions list
create materialized view all_missions as (
  select m.id,
    m.start_time,
    m.mission_name,
    m.mission_name_source,
    m.briefing_name,
    m.on_load_name,
    m.author,
    m.server_name,
    m.server_profile,
    m.capture_delay,
    m.addon_version,
    m.extension_version,
    m.extension_build,
    m.ocap_recorder_extension_version,
    m.tag,
    json_build_object(
      'west',
      m.playable_west,
      'east',
      m.playable_east,
      'independent',
      m.playable_independent,
      'civilian',
      m.playable_civilian,
      'logic',
      m.playable_logic
    ) as playable_slots,
    json_build_object(
      'eastWest',
      m.sidefriendly_east_west,
      'eastIndependent',
      m.sidefriendly_east_independent,
      'westIndependent',
      m.sidefriendly_west_independent
    ) as side_friendly,
    json_agg(
      json_build_object(
        'name',
        am.addon_name,
        'workshop_id',
        am.addon_workshop_id
      )
    ) as addons,
    w.world_name,
    w.display_name,
    w.world_size,
    ST_AsText(
      ST_Transform(
        ST_SetSRID(w.location::geometry, 3857),
        4326
      )
    ) as world_location,
    w.author as world_author,
    ss.last_frame::int
  from missions m
    left join worlds w on m.world_id = w.id
    join addons_to_missions am on am.mission_id = m.id
    left join (
      select mission_id,
        MAX(capture_frame) as last_frame
      from soldier_states
      Group by mission_id
    ) ss on ss.mission_id = m.id
  group by m.id,
    w.id,
    ss.last_frame
  order by m.start_time DESC
);
-- firelines
create materialized view projectile_paths_geojson as (
  select p.mission_id::int,
    p.capture_frame::int,
    json_build_object(
      'type',
      'FeatureCollection',
      'features',
      json_agg(
        json_build_object(
          'geometry',
          ST_AsGeoJSON(
            ST_Transform(
              ST_SetSRID(p.positions::geometry, 3857),
              4326
            )
          )::json,
          'properties',
          json_build_object(
            'mission_id',
            p.mission_id,
            'capture_frame',
            p.capture_frame,
            'firer_side',
            s.side,
            'weapon',
            p.weapon,
            'weapon_display',
            p.weapon_display,
            'magazine',
            p.magazine,
            'magazine_display',
            p.magazine_display,
            'muzzle',
            p.muzzle,
            'muzzle_display',
            p.muzzle_display,
            'ammo',
            p.ammo,
            'mode',
            p.mode,
            'distance',
            ST_3DDistance(
              st_startpoint(p.positions::geometry),
              st_endpoint(p.positions::geometry)
            )::float
          )
        )
      )
    )::json as geojson
  from projectile_events p
    left join (
      select id,
        side
      from soldiers
      order by id
    ) s on s.id = p.firer_id
  where p.capture_frame is not null
  group by p.mission_id,
    p.capture_frame
);
-- hitlocations and frames
create materialized view hit_locations_geojson as (
  select pe.mission_id::int,
    ph.capture_frame::int,
    json_build_object(
      'type',
      'FeatureCollection',
      'features',
      json_agg(
        json_build_object(
          'geometry',
          ST_AsGeoJSON(
            ST_Transform(
              ST_SetSRID(ph.position::geometry, 3857),
              4326
            )
          )::json,
          'properties',
          json_build_object(
            'mission_id',
            pe.mission_id,
            'capture_frame',
            ph.capture_frame
          )
        )
      )
    )::json as geojson
  from projectile_events pe
    left join (
      select capture_frame,
        position,
        projectile_event_id
      from projectile_hits_soldiers
      union
      select capture_frame,
        position,
        projectile_event_id
      from projectile_hits_vehicles
      order by capture_frame
    ) ph on pe.id = ph.projectile_event_id
  where ph.capture_frame is not null
  group by pe.mission_id,
    ph.capture_frame
);
-- soldier hit events
create materialized view hit_soldiers as (
  select p.mission_id,
    h.capture_frame,
    ST_AsText(
      ST_Transform(
        ST_SetSRID(h.position::geometry, 3857),
        4326
      )
    ) as position,
    s.fired_unit_ocap_id,
    s.fired_unit_uid,
    s.fired_unit_name,
    s.fired_unit_side,
    s.fired_unit_isplayer,
    s.fired_unit_type,
    vehicle.vehicle_ocap_id,
    vehicle.vehicle_name,
    p.used_weapon,
    p.used_magazine,
    victim.victim_ocap_id,
    victim.victim_unit_uid,
    victim.victim_unit_name,
    victim.victim_unit_side,
    victim.victim_unit_isplayer,
    victim.victim_unit_type,
    h.components_hit
  from public.projectile_hits_soldiers as h
    left join (
      select id,
        mission_id,
        firer_id,
        vehicle_id,
        weapon_display as used_weapon,
        magazine_display as used_magazine
      from projectile_events
      order by id
    ) p on p.id = h.projectile_event_id
    left join (
      select id,
        ocap_id as fired_unit_ocap_id,
        player_uid as fired_unit_uid,
        unit_name as fired_unit_name,
        side as fired_unit_side,
        is_player as fired_unit_isplayer,
        display_name as fired_unit_type
      from soldiers
      order by id
    ) s on s.id = p.firer_id
    left join (
      select id,
        ocap_id as vehicle_ocap_id,
        display_name as vehicle_name
      from vehicles
      order by id
    ) vehicle on vehicle.id = p.vehicle_id
    left join (
      select id,
        ocap_id as victim_ocap_id,
        player_uid as victim_unit_uid,
        unit_name as victim_unit_name,
        side as victim_unit_side,
        is_player as victim_unit_isplayer,
        display_name as victim_unit_type
      from soldiers
      order by id
    ) victim on victim.id = h.soldier_id
);
-- vehicle hit events
create materialized view hit_vehicles as (
  select p.mission_id,
    h.capture_frame,
    ST_AsText(
      ST_Transform(
        ST_SetSRID(h.position::geometry, 3857),
        4326
      )
    ) as position,
    s.fired_unit_ocap_id,
    s.fired_unit_uid,
    s.fired_unit_name,
    s.fired_unit_side,
    s.fired_unit_isplayer,
    s.fired_unit_type,
    vehicle.vehicle_ocap_id,
    vehicle.vehicle_name,
    p.used_weapon,
    p.used_magazine,
    victim.victim_ocap_id,
    victim.victim_unit_type,
    h.components_hit
  from public.projectile_hits_vehicles as h
    left join (
      select id,
        mission_id,
        firer_id,
        vehicle_id,
        weapon_display as used_weapon,
        magazine_display as used_magazine
      from projectile_events
    ) p on p.id = h.projectile_event_id
    left join (
      select id,
        ocap_id as fired_unit_ocap_id,
        player_uid as fired_unit_uid,
        unit_name as fired_unit_name,
        side as fired_unit_side,
        is_player as fired_unit_isplayer,
        display_name as fired_unit_type
      from soldiers
    ) s on s.id = p.firer_id
    left join (
      select id,
        ocap_id as vehicle_ocap_id,
        display_name as vehicle_name
      from vehicles
    ) vehicle on vehicle.id = p.vehicle_id
    left join (
      select id,
        ocap_id as victim_ocap_id,
        display_name as victim_unit_type
      from vehicles
    ) victim on victim.id = h.vehicle_id
);
-- soldier states
create materialized view soldier_frames AS (
  select e.mission_id,
    e.ocap_id,
    ss.time,
    ss.capture_frame,
    ST_AsText(
      ST_Transform(
        ST_SetSRID(ss.position::geometry, 3857),
        4326
      )
    ) as position,
    ss.bearing,
    ss.lifestate,
    ss.unit_name,
    ss.is_player,
    ss.current_role,
    ss.has_stable_vitals,
    ss.is_dragged_carried,
    ss.stance,
    json_build_object(
      'infantry',
      ss.scores_infantry_kills,
      'vehicle',
      ss.scores_vehicle_kills,
      'armor',
      ss.scores_armor_kills,
      'air',
      ss.scores_air_kills,
      'deaths',
      ss.scores_deaths,
      'total',
      ss.scores_total_score
    ) as scores,
    ss.in_vehicle,
    veh.vehicle_ocap_id,
    veh.vehicle_name,
    veh.vehicle_class
  from soldiers e
    left join (
      select *
      from soldier_states
    ) ss on ss.soldier_id = e.id
    left join (
      select id,
        ocap_id as vehicle_ocap_id,
        display_name as vehicle_name,
        class_name as vehicle_class
    ) veh on veh.id = ss.in_vehicle_object_id
  order by e.mission_id,
    ss.capture_frame,
    e.ocap_id
);
-- soldier frames geojson
create materialized view soldier_frames_geojson as (
  select e.mission_id::int,
    ss.capture_frame::int,
    --	e.ocap_id,
    jsonb_build_object(
      'type',
      'FeatureCollection',
      'features',
      jsonb_agg(
        jsonb_build_object(
          'type',
          'Feature',
          'id',
          ss.id,
          'geometry',
          ST_AsGeoJSON(
            st_transform(
              ST_SetSRID(ss.position::geometry, 3857),
              4326
            )
          )::jsonb,
          'properties',
          jsonb_build_object(
            'mission_id',
            e.mission_id,
            'ocap_id',
            e.ocap_id,
            'join_frame',
            e.join_frame,
            'time',
            ss.time,
            'capture_frame',
            ss.capture_frame,
            'ocap_type',
            e.ocap_type,
            'side',
            e.side,
            'bearing',
            ss.bearing,
            'lifestate',
            ss.lifestate,
            'unit_name',
            ss.unit_name,
            'is_player',
            ss.is_player,
            'role',
            ss.current_role,
            'has_stable_vitals',
            ss.has_stable_vitals,
            'is_dragged_carried',
            ss.is_dragged_carried,
            'stance',
            ss.stance,
            'scores',
            jsonb_build_object(
              'infantry',
              ss.scores_infantry_kills,
              'vehicle',
              ss.scores_vehicle_kills,
              'armor',
              ss.scores_armor_kills,
              'air',
              ss.scores_air_kills,
              'deaths',
              ss.scores_deaths,
              'total',
              ss.scores_total_score
            ),
            'is_in_vehicle',
            ss.in_vehicle
          )
        )
      )
    )::json as geojson
  from soldiers e
    left join (
      select *
      from soldier_states
    ) ss on ss.soldier_id = e.id
  group by e.mission_id,
    ss.capture_frame
  order by e.mission_id,
    ss.capture_frame
);
-- vehicle frames geojson
create materialized view vehicle_frames_geojson as (
  select e.mission_id::int,
    ss.capture_frame::int,
    --	e.ocap_id,
    jsonb_build_object(
      'type',
      'FeatureCollection',
      'features',
      jsonb_agg(
        jsonb_build_object(
          'type',
          'Feature',
          'id',
          ss.id,
          'geometry',
          ST_AsGeoJSON(
            st_transform(
              ST_SetSRID(ss.position::geometry, 3857),
              4326
            )
          )::jsonb,
          'properties',
          jsonb_build_object(
            'mission_id',
            e.mission_id,
            'ocap_id',
            e.ocap_id,
            'join_frame',
            e.join_frame,
            'time',
            ss.time,
            'capture_frame',
            ss.capture_frame,
            'ocap_type',
            e.ocap_type,
            'class_name',
            e.class_name,
            'display_name',
            e.display_name,
            'side',
            ss.side,
            'bearing',
            ss.bearing,
            'is_alive',
            ss.is_alive,
            'crew',
            ss.crew,
            'fuel',
            ss.fuel,
            'damage',
            ss.damage,
            'locked',
            ss.locked,
            'engine_on',
            ss.engine_on,
            'side',
            ss.side,
            'vector',
            jsonb_build_object(
              'vector_dir',
              ss.vector_dir,
              'vector_up',
              ss.vector_up
            ),
            'turret',
            jsonb_build_object(
              'turret_azimuth',
              ss.turret_azimuth,
              'turret_elevation',
              ss.turret_elevation
            )
          )
        )
      )
    )::json as geojson
  from vehicles e
    left join (
      select *
      from vehicle_states
    ) ss on ss.vehicle_id = e.id
  group by e.mission_id,
    ss.capture_frame
  order by e.mission_id,
    ss.capture_frame
);
-- markers view for mission playback
create materialized view mission_markers as (
  select
    m.id as marker_id,
    m.mission_id,
    m.marker_name,
    m.marker_type,
    m.text,
    m.color,
    m.size,
    m.side,
    m.shape,
    m.brush,
    m.alpha,
    m.direction,
    m.capture_frame as created_frame,
    m.is_deleted,
    ST_AsText(
      ST_Transform(
        ST_SetSRID(m.position::geometry, 3857),
        4326
      )
    ) as position,
    json_agg(
      json_build_object(
        'frame', ms.capture_frame,
        'position', ST_AsText(ST_Transform(ST_SetSRID(ms.position::geometry, 3857), 4326)),
        'direction', ms.direction,
        'alpha', ms.alpha
      ) order by ms.capture_frame
    ) filter (where ms.id is not null) as states
  from markers m
  left join marker_states ms on ms.marker_id = m.id
  group by m.id
  order by m.capture_frame
);

-- refresh function for markers
create or replace function refresh_mission_markers()
returns trigger as $$
begin
  refresh materialized view concurrently mission_markers;
  return null;
end;
$$ language plpgsql;