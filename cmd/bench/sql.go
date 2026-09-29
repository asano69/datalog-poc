package main

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/google/mangle/ast"
	"github.com/google/mangle/parse"
	_ "modernc.org/sqlite"
)

// sqlBench stands in for the card_links collection backed by SQLite, with
// the same two indexes the collection has (one on source, one on target).
type sqlBench struct {
	db            *sql.DB
	join, own, in *sql.Stmt
}

func cardName(i int) string { return fmt.Sprintf("card%d", i) }

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func newSQLBench(g *graph) *sqlBench {
	db, err := sql.Open("sqlite", ":memory:")
	check(err)
	// An in-memory database exists per connection, so use exactly one.
	db.SetMaxOpenConns(1)

	_, err = db.Exec(`CREATE TABLE card_links (source TEXT NOT NULL, target TEXT NOT NULL)`)
	check(err)

	tx, err := db.Begin()
	check(err)
	insert, err := tx.Prepare(`INSERT INTO card_links (source, target) VALUES (?, ?)`)
	check(err)
	for from, targets := range g.out {
		for _, to := range targets {
			_, err = insert.Exec(cardName(from), cardName(to))
			check(err)
		}
	}
	check(tx.Commit())

	_, err = db.Exec(`CREATE INDEX idx_source ON card_links (source)`)
	check(err)
	_, err = db.Exec(`CREATE INDEX idx_target ON card_links (target)`)
	check(err)

	b := &sqlBench{db: db}
	b.join = prepare(db, `SELECT DISTINCT b.source FROM card_links a
		JOIN card_links b ON b.target = a.target
		WHERE a.source = ? AND b.source != a.source`)
	b.own = prepare(db, `SELECT target FROM card_links WHERE source = ?`)
	b.in = prepare(db, `SELECT source FROM card_links WHERE target = ?`)
	return b
}

func prepare(db *sql.DB, query string) *sql.Stmt {
	stmt, err := db.Prepare(query)
	check(err)
	return stmt
}

// queryStrings reads the whole result before returning: with a single
// connection, the next query would block on an unclosed rows.
func queryStrings(stmt *sql.Stmt, arg string) []string {
	rows, err := stmt.Query(arg)
	check(err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		check(rows.Scan(&s))
		out = append(out, s)
	}
	check(rows.Err())
	return out
}

// queryJoin answers links2hop with one SQL join. It reports no loaded facts.
func (b *sqlBench) queryJoin(cardID int) (results, loaded int) {
	return len(queryStrings(b.join, cardName(cardID))), 0
}

// queryMangle loads the same facts as loadFacts, but through SQL queries
// (own targets first, then the inbound links of each target), and then
// evaluates the rule with Mangle.
func (b *sqlBench) queryMangle(kind string, unit parse.SourceUnit, cardID int) (results, loaded int) {
	name := cardName(cardID)
	store := newStore(kind)
	for _, target := range queryStrings(b.own, name) {
		if store.Add(ast.NewAtom("own", ast.String(target))) {
			loaded++
		}
		for _, other := range queryStrings(b.in, target) {
			if other != name && store.Add(ast.NewAtom("link", ast.String(other), ast.String(target))) {
				loaded++
			}
		}
	}
	return evaluate(unit, store), loaded
}
