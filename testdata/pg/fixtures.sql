-- FK dependency fixture for pg handler tests (schema app).
-- Alphabetical order would be: accounts, posts, users
-- FK order must be: accounts -> users -> posts

CREATE SCHEMA IF NOT EXISTS app;
CREATE SCHEMA IF NOT EXISTS roundtrip;

DROP TABLE IF EXISTS roundtrip.posts;
DROP TABLE IF EXISTS roundtrip.users;
DROP TABLE IF EXISTS roundtrip.accounts;

DROP TABLE IF EXISTS app.posts;
DROP TABLE IF EXISTS app.users;
DROP TABLE IF EXISTS app.accounts;

CREATE TABLE app.accounts (
    id   serial PRIMARY KEY,
    name text NOT NULL
);

CREATE TABLE app.users (
    id         serial PRIMARY KEY,
    account_id int NOT NULL REFERENCES app.accounts (id),
    email      text NOT NULL
);

CREATE TABLE app.posts (
    id      serial PRIMARY KEY,
    user_id int  NOT NULL REFERENCES app.users (id),
    title   text NOT NULL
);

INSERT INTO app.accounts (name) VALUES ('acme'), ('beta');
INSERT INTO app.users (account_id, email) VALUES
    (1, 'alice@example.com'),
    (1, 'bob@example.com'),
    (2, 'carol@example.com');
INSERT INTO app.posts (user_id, title) VALUES
    (1, 'hello'),
    (2, 'world'),
    (3, 'fk test');

-- Simple public table for default schema tests
CREATE TABLE IF NOT EXISTS public.simple (
    id   serial PRIMARY KEY,
    note text NOT NULL
);

TRUNCATE public.simple RESTART IDENTITY;
INSERT INTO public.simple (note) VALUES ('one'), ('two');
