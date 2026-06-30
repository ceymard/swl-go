package sqlite

import (
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

const driverName = "sqlite3"

func dsnReadOnly(path string) string {
	return fmt.Sprintf("file:%s?mode=ro", path)
}

func dsnReadWrite(path string) string {
	return fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL", path)
}
