// Command query loads triple-style facts from data/facts.csv into a Mangle
// fact store, evaluates the rules in mangle/rules.mg against those facts,
// and prints the results of a few example queries.
package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"log"
	"os"

	"github.com/google/mangle/ast"
	"github.com/google/mangle/factstore"
	"github.com/google/mangle/interpreter"
	"github.com/google/mangle/parse"
)

// loadTriplesCSV reads a CSV file with header "subject,predicate,object" and
// adds one fact per row to store. The CSV "predicate" column becomes the
// Mangle predicate name, so a row like "alice,parent,bob" becomes the fact
// parent(alice, bob).
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
	}
	return nil
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
	// 1. Build a fact store and populate it with facts from the CSV file.
	store := factstore.NewSimpleInMemoryStore()
	if err := loadTriplesCSV("data/facts.csv", store); err != nil {
		log.Fatalf("loading facts: %v", err)
	}

	// 2. Parse the rules file. It only contains a Decl for "parent" plus
	// rules; the "parent" facts themselves already live in the store.
	rulesFile, err := os.Open("mangle/rules.mg")
	if err != nil {
		log.Fatalf("opening rules file: %v", err)
	}
	defer rulesFile.Close()
	unit, err := parse.Unit(bufio.NewReader(rulesFile))
	if err != nil {
		log.Fatalf("parsing rules file: %v", err)
	}

	// 3. Preload the interpreter with our fact store and the parsed rules,
	// then evaluate: this derives "sibling" and "ancestor" facts and adds
	// them to the store.
	interp := interpreter.New(os.Stdout, ".", nil)
	knownPredicates := map[ast.PredicateSym]ast.Decl{}
	if err := interp.Preload([]parse.SourceUnit{unit}, store, knownPredicates); err != nil {
		log.Fatalf("evaluating rules: %v", err)
	}

	// 4. Query the results.
	runQuery(interp, "parent(_, _)")
	runQuery(interp, "sibling(_, _)")
	runQuery(interp, "ancestor(_, _)")
	runQuery(interp, `ancestor("alice", "erin")`)
}
