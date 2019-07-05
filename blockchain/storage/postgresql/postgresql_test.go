package postgresql

import (
	"database/sql"
	"io/ioutil"
	"strings"
	"testing"

	_ "github.com/lib/pq"
	"github.com/umbracle/minimal/blockchain/storage"

	"github.com/hashicorp/go-hclog"
)

const endpoint = "user=postgres password=docker sslmode=disable"

func checkPostgreSQLRunning() bool {
	db, err := sql.Open("postgres", endpoint)
	if err != nil {
		panic(err)
	}
	if err = db.Ping(); err != nil {
		return false
	}
	return true
}

func buildDB(t *testing.T) func() {
	db, err := sql.Open("postgres", endpoint)
	if err != nil {
		panic(err)
	}
	if err = db.Ping(); err != nil {
		t.Skip("PostgreSQL is not running")
	}

	// build tables
	data, err := ioutil.ReadFile("./schema/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(data)); err != nil {
		t.Fatal(err)
	}

	// remove the tables with a callback
	rows, err := db.Query("select table_name from information_schema.tables where table_schema = 'public'")
	if err != nil {
		t.Fatal(err)
	}

	tables := []string{}
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}

	closeFn := func() {
		defer db.Close()
		if _, err := db.Exec("DROP TABLE " + strings.Join(tables, ",") + " CASCADE;"); err != nil {
			t.Fatal(err)
		}
	}
	return closeFn
}

func newBackend(t *testing.T) *Backend {
	c := map[string]interface{}{
		"endpoint": endpoint,
	}
	logger := hclog.New(&hclog.LoggerOptions{
		Output: ioutil.Discard,
	})
	b, err := Factory(c, logger)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newStorage(t *testing.T) (storage.Storage, func()) {
	close := buildDB(t)
	b := newBackend(t)
	return b, close
}

func TestStorage(t *testing.T) {
	if !checkPostgreSQLRunning() {
		t.Skip("PostgreSQL is not running")
	}
	storage.TestStorage(t, newStorage)
}
