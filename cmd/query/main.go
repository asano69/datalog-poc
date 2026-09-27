// Command query loads triple-style facts from a CSV file into a Mangle fact
// store, evaluates a Mangle rules (.mg) file against those facts, and runs
// the queries listed in a queries file, printing the results of each. The
// facts file, rules file, and queries file can all be overridden from the
// command line; see the -h flag for details.
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/mangle/ast"
	"github.com/google/mangle/factstore"
	"github.com/google/mangle/interpreter"
	"github.com/google/mangle/parse"
)

// loadTriplesCSV reads a CSV file with header "subject,predicate,object" and
// adds two facts per row to store:
//   - a fact using the "predicate" column as the Mangle predicate name, so a
//     row like "alice,parent,bob" becomes parent(alice, bob). This is the
//     convenient, relation-specific view used by rules like sibling/ancestor.
//   - a generic triple(Subject, Predicate, Object) fact, e.g.
//     triple(alice, parent, bob). This lets rules reason across *any*
//     relation generically (see links2hop in mangle/rules.mg), without
//     needing to know the relation's name ahead of time.
func loadTriplesCSV(path string, store factstore.FactStoreWithRemove) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(bufio.NewReader(f))
	rows, err := r.ReadAll()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("%s: empty file", path)
	}

	for _, row := range rows[1:] { // skip header row
		subject, predicate, object := row[0], row[1], row[2]
		atom := ast.NewAtom(predicate, ast.String(subject), ast.String(object))
		store.Add(atom)
		triple := ast.NewAtom("triple", ast.String(subject), ast.String(predicate), ast.String(object))
		store.Add(triple)
	}
	return nil
}

// loadQueries reads a file with one query per line. Blank lines and lines
// starting with "#" are ignored, so a queries file can be commented like a
// .mg file.
func loadQueries(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var queries []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		queries = append(queries, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return queries, nil
}

func runQuery(interp *interpreter.Interpreter, query string) {
	atom, err := interp.ParseQuery(query)
	if err != nil {
		log.Fatalf("parsing query %q: %v", query, err)
	}
	facts, err := interp.Query(atom)
	if err != nil {
		log.Fatalf("running query %q: %v", query, err)
	}
	fmt.Printf("? %s\n", query)
	for _, fact := range facts {
		fmt.Printf("  %s\n", fact.DisplayString())
	}
	fmt.Printf("  (%d results)\n\n", len(facts))
}

func main() {
	factsPath := flag.String("facts", "data/facts.csv", "path to a triple-style CSV file (subject,predicate,object)")
	rulesPath := flag.String("rules", "mangle/rules.mg", "path to a Mangle rules (.mg) file")
	queriesPath := flag.String("queries", "mangle/queries.txt", "path to a file with one query per line (# comments and blank lines are ignored)")
	flag.Parse()

	// Any positional arguments after the flags override the queries file
	// entirely, for quick one-off tests, e.g.:
	//   go run ./cmd/query -rules mangle/other.mg 'foo(_, _)' 'bar("x", _)'
	queries := flag.Args()
	if len(queries) == 0 {
		var err error
		queries, err = loadQueries(*queriesPath)
		if err != nil {
			log.Fatalf("loading queries: %v", err)
		}
	}

	// 1. Build a fact store and populate it with facts from the CSV file.
	store := factstore.NewSimpleInMemoryStore()
	if err := loadTriplesCSV(*factsPath, store); err != nil {
		log.Fatalf("loading facts: %v", err)
	}

	// 2. Parse the rules file. It only contains a Decl for "parent" plus
	// rules; the "parent" facts themselves already live in the store.
	rulesFile, err := os.Open(*rulesPath)
	if err != nil {
		log.Fatalf("opening rules file: %v", err)
	}
	defer rulesFile.Close()
	unit, err := parse.Unit(bufio.NewReader(rulesFile))
	if err != nil {
		log.Fatalf("parsing rules file: %v", err)
	}

	// 3. Preload the interpreter with our fact store and the parsed rules,
	// then evaluate: this derives new facts (e.g. "sibling", "ancestor" in
	// the default rules file) and adds them to the store.
	interp := interpreter.New(os.Stdout, ".", nil)
	knownPredicates := map[ast.PredicateSym]ast.Decl{}
	if err := interp.Preload([]parse.SourceUnit{unit}, store, knownPredicates); err != nil {
		log.Fatalf("evaluating rules: %v", err)
	}

	// 4. Query the results.
	for _, query := range queries {
		runQuery(interp, query)
	}
}
