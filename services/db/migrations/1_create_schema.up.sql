CREATE EXTENSION IF NOT EXISTS postgis;
CREATE SCHEMA earthquake;
CREATE TABLE IF NOT EXISTS earthquake.events
(
  id varchar(100) NOT NULL,
  coordinates geography(POINT, 4326),
  magnitude numeric(4, 2) CHECK(magnitude >= 0 AND magnitude <= 10.0) DEFAULT 0,
  place varchar(250) NOT NULL,
  time timestamp NOT NULL,
  updated timestamp NOT NULL,
  tz smallint,
  url text,
  detail text,
  felt smallint DEFAULT 0,
  cdi numeric(4, 2) CHECK(cdi >= 0 AND cdi <= 10.0) DEFAULT 0, 
  mmi numeric(4, 2) CHECK(mmi >= 0 AND mmi <= 10.0),
  alert varchar(8),
  status varchar(10),
  significance smallint CHECK(significance >= 0 AND significance <= 1000) DEFAULT 0,
  network text,
  code varchar(50),
  ids text,
  sources text,
  types text,
  nst smallint CHECK(nst >= 0),
  dmin numeric(5, 2) CHECK(dmin >= 0),
  rms numeric(5, 2) CHECK(rms >= 0),
  gap smallint CHECK(gap >= 0),
  magnitude_type varchar(20),
  type varchar(20),
  title text,
  dmin_distance numeric(7, 2),
  PRIMARY KEY(id, time)
) PARTITION BY RANGE (time);

CREATE TABLE IF NOT EXISTS earthquake.impact
(
  event_id varchar(100) NOT NULL,
  event_time timestamp NOT NULL,
  updated timestamp NOT NULL,
  data text,
  PRIMARY KEY (event_id, event_time),
  FOREIGN KEY (event_id, event_time) REFERENCES earthquake.events(id, time)
);

