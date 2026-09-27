# rules.mg
#
# "parent" and "triple" are extensional predicates: their facts come from
# data/facts.csv, loaded by cmd/query/main.go, not from clauses in this file.
# We still need to declare them so the analyzer knows their arity when they
# appear in rule bodies.
Decl parent(Parent, Child).
Decl triple(Subject, Predicate, Object).

# sibling(X, Y): X and Y share a parent, and are not the same person.
sibling(X, Y) :- parent(P, X), parent(P, Y), X != Y.

# ancestor(X, Y): X is an ancestor of Y.
# Base case: every parent is an ancestor.
ancestor(X, Y) :- parent(X, Y).
# Recursive case: a parent of an ancestor is also an ancestor.
ancestor(X, Z) :- parent(X, Y), ancestor(Y, Z).

# links2hop(A, B): "2-hop" link inference. If A and B both point to the same
# target X via the *same* kind of relation P (e.g. both are tagged /fruit),
# then A and B are considered linked, in both directions.
#   a --P--> x <--P-- b   =>   links2hop(a, b) and links2hop(b, a)
# Requiring P to be shared (not just X) keeps this to "same relation type";
# drop the shared P (use two separate variables) if you want any relation
# type to count as a link, not just matching ones.
links2hop(A, B) :- triple(A, P, X), triple(B, P, X), A != B.
