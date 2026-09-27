# rules.mg
#
# "parent" is an extensional predicate: its facts come from data/facts.csv,
# loaded by cmd/query/main.go, not from clauses in this file. We still need
# to declare it so the analyzer knows its arity when it appears in rule bodies.
Decl parent(Parent, Child).

# sibling(X, Y): X and Y share a parent, and are not the same person.
sibling(X, Y) :- parent(P, X), parent(P, Y), X != Y.

# ancestor(X, Y): X is an ancestor of Y.
# Base case: every parent is an ancestor.
ancestor(X, Y) :- parent(X, Y).
# Recursive case: a parent of an ancestor is also an ancestor.
ancestor(X, Z) :- parent(X, Y), ancestor(Y, Z).
