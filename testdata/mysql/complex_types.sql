-- Complex JSON columns for mysql handler tests (MySQL 8+).

DROP TABLE IF EXISTS documents;

CREATE TABLE documents (
    id      INT AUTO_INCREMENT PRIMARY KEY,
    tags    JSON NOT NULL,
    payload JSON NOT NULL
);

INSERT INTO documents (tags, payload) VALUES (
    JSON_ARRAY('alpha', 'beta'),
    JSON_OBJECT(
        'users', JSON_ARRAY(
            JSON_OBJECT(
                'id', 1,
                'tags', JSON_ARRAY('a', 'b'),
                'profile', JSON_OBJECT('active', true)
            )
        ),
        'meta', JSON_OBJECT(
            'count', 2,
            'nested', JSON_OBJECT('ok', true, 'labels', JSON_ARRAY('x', 'y'))
        )
    )
);
