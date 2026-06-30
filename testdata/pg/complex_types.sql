-- Complex column types for pg handler tests.

CREATE SCHEMA IF NOT EXISTS complex;

DROP TABLE IF EXISTS complex.documents;

CREATE TABLE complex.documents (
    id      serial PRIMARY KEY,
    tags    text[] NOT NULL,
    payload jsonb  NOT NULL
);

INSERT INTO complex.documents (tags, payload) VALUES
(
    ARRAY['alpha', 'beta'],
    '{
        "users": [
            {"id": 1, "tags": ["a", "b"], "profile": {"active": true}}
        ],
        "meta": {
            "count": 2,
            "nested": {"ok": true, "labels": ["x", "y"]}
        }
    }'::jsonb
);
