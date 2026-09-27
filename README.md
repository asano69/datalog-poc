# datalog-poc

```shell
go run ./cmd/query                                   # facts/rules/queriesすべてデフォルト
go run ./cmd/query -queries mangle/other_queries.txt # クエリだけ差し替え
go run ./cmd/query -rules mangle/other.mg 'foo(_,_)' # 一時的にクエリを直接指定(ファイルは無視)
```
