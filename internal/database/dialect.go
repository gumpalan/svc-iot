package database

import (
	"strconv"
	"strings"
)

// Rebind mengubah placeholder query (?) menjadi placeholder spesifik dialek ($1, $2, ...)
// hanya untuk dialek PostgreSQL. Dialek SQLite, MySQL, dan MariaDB tetap menggunakan (?).
// String literal berpetik tunggal diabaikan agar tidak salah mengganti karakter tanda tanya di dalam teks.
func Rebind(dialect Dialect, query string) string {
	if dialect != DialectPgSQL {
		return query
	}

	var buf strings.Builder
	buf.Grow(len(query) + 16)

	argIndex := 1
	inString := false

	for i := 0; i < len(query); i++ {
		ch := query[i]

		if ch == '\'' {
			// Periksa escape '' di SQL
			if inString && i+1 < len(query) && query[i+1] == '\'' {
				buf.WriteByte('\'')
				buf.WriteByte('\'')
				i++
				continue
			}
			inString = !inString
			buf.WriteByte(ch)
			continue
		}

		if ch == '?' && !inString {
			buf.WriteByte('$')
			buf.WriteString(strconv.Itoa(argIndex))
			argIndex++
			continue
		}

		buf.WriteByte(ch)
	}

	return buf.String()
}
