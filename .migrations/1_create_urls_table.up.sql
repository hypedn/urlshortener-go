CREATE TABLE urls (
    short_code TEXT PRIMARY KEY,
    long_url TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_urls_long_url ON urls (long_url);
