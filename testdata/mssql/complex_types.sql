-- Complex JSON columns for mssql handler tests (SQL Server 2016+).

IF OBJECT_ID(N'dbo.documents', N'U') IS NOT NULL DROP TABLE dbo.documents;

CREATE TABLE dbo.documents (
    id      INT IDENTITY(1,1) PRIMARY KEY,
    tags    NVARCHAR(MAX) NOT NULL,
    payload NVARCHAR(MAX) NOT NULL
);

INSERT INTO dbo.documents (tags, payload) VALUES (
    N'["alpha","beta"]',
    N'{"users":[{"id":1,"tags":["a","b"],"profile":{"active":true}}],"meta":{"count":2,"nested":{"ok":true,"labels":["x","y"]}}}'
);
