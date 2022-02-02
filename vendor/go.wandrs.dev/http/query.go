package http

// FIXME: We should differ Query and Form, currently we just use form as query
// Currently to be compatible with macaron, we keep it.

// Query returns request form as string with default
func (r *request) Query(key string, defaults ...string) string {
	return (*Forms)(r.req).MustString(key, defaults...)
}

// QueryTrim returns request form as string with default and trimmed spaces
func (r *request) QueryTrim(key string, defaults ...string) string {
	return (*Forms)(r.req).MustTrimmed(key, defaults...)
}

// QueryStrings returns request form as strings with default
func (r *request) QueryStrings(key string, defaults ...[]string) []string {
	return (*Forms)(r.req).MustStrings(key, defaults...)
}

// QueryInt returns request form as int with default
func (r *request) QueryInt(key string, defaults ...int) int {
	return (*Forms)(r.req).MustInt(key, defaults...)
}

// QueryInt64 returns request form as int64 with default
func (r *request) QueryInt64(key string, defaults ...int64) int64 {
	return (*Forms)(r.req).MustInt64(key, defaults...)
}

// QueryBool returns request form as bool with default
func (r *request) QueryBool(key string, defaults ...bool) bool {
	return (*Forms)(r.req).MustBool(key, defaults...)
}
