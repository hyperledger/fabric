package testdb

import (
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql" // register mysql driver
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"           // register postgresql driver
	_ "github.com/mattn/go-sqlite3" // register sqlite3 driver
)

const (
	mysqlTruncateTables = `
TRUNCATE certificates;
TRUNCATE ocsp_responses;
`

	pgTruncateTables = `
CREATE OR REPLACE FUNCTION truncate_tables() RETURNS void AS $$
DECLARE
    statements CURSOR FOR
        SELECT tablename FROM pg_tables
        WHERE tablename != 'goose_db_version'
          AND tableowner = session_user
          AND schemaname = 'public';
BEGIN
    FOR stmt IN statements LOOP
        EXECUTE 'TRUNCATE TABLE ' || quote_ident(stmt.tablename) || ' CASCADE;';
    END LOOP;
END;
$$ LANGUAGE plpgsql;

SELECT truncate_tables();
`

	sqliteTruncateTables = `
DELETE FROM certificates;
DELETE FROM ocsp_responses;
`
)

// MySQLDB returns a MySQL db instance for certdb testing.
func MySQLDB() *sqlx.DB {
	connStr := "root@tcp(localhost:3306)/certdb_development?parseTime=true"

	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		connStr = dbURL
	}

	db, err := sqlx.Open("mysql", connStr)
	if err != nil {
		panic(err)
	}

	Truncate(db)

	return db
}

// PostgreSQLDB returns a PostgreSQL db instance for certdb testing.
func PostgreSQLDB() *sqlx.DB {
	connStr := "dbname=certdb_development sslmode=disable"

	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		connStr = dbURL
	}

	db, err := sqlx.Open("postgres", connStr)
	if err != nil {
		panic(err)
	}

	Truncate(db)

	return db
}

// SQLiteDB returns a SQLite db instance for certdb testing.
func SQLiteDB(dbpath string) *sqlx.DB {
	db, err := sqlx.Open("sqlite3", dbpath)
	if err != nil {
		panic(err)
	}

	Truncate(db)

	return db
}

// Truncate truncates the DB
func Truncate(db *sqlx.DB) {
	var sql []string
	switch db.DriverName() {
	case "mysql":
		sql = strings.Split(mysqlTruncateTables, "\n")
	case "postgres":
		sql = []string{pgTruncateTables}
	case "sqlite3":
		sql = []string{sqliteTruncateTables}
	default:
		panic("Unknown driver")
	}

	for _, expr := range sql {
		if len(strings.TrimSpace(expr)) == 0 {
			continue
		}
		if _, err := db.Exec(expr); err != nil {
			panic(err)
		}
	}
}
