# datalog-poc

```shell
go run ./cmd/query                                   # facts/rules/queriesすべてデフォルト
go run ./cmd/query -queries mangle/other_queries.txt # クエリだけ差し替え
go run ./cmd/query -rules mangle/other.mg 'foo(_,_)' # 一時的にクエリを直接指定(ファイルは無視)
```

## ベンチマーク
- 合成データはリンク数が平均3で、リンク先はハブ寄りに偏らせます。

(a) 全件実体化の時間とメモリ(Preload)
(b) 特定ページ 1 件からの検索の応答時間
(c) 両方。可能ならこの場合、(b) は「事前に全件実体化する」方式と「クエリごとに必要な分だけ計算する」方式の比較も含める

- 述語は1種類で、(a) と (b) の両方を測ります。
- (b) は「実体化済み」と「オンデマンド」を比較し、規模は 1k / 10k / 100k で、Mangle のみを対象にします。

```
go run ./cmd/bench -n 1000
go run ./cmd/bench -n 10000 -store simple
go run ./cmd/bench -n 100000 -materialize=false   # オンデマンドのみ
go run ./cmd/bench -n 100000                       # 実体化も測る(時間とメモリに注意)
```

