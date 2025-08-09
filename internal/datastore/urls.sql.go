package datastore

const (
	insertURL = `
	INSERT INTO urls (short_code, long_url)
	VALUES (@short_code, @long_url)
	ON CONFLICT (short_code) DO NOTHING
	RETURNING *
	`

	getURL = `
	SELECT long_url FROM urls
	WHERE short_code = $1
	`
)
