package main

import (
	ardgo "github.com/akonwi/ard/go"
	decode "programs/__ard_stdlib/decode"
	fs "programs/__ard_stdlib/fs"
	io "programs/__ard_stdlib/io"
	sql "programs/__ard_stdlib/sql"
	strconv "strconv"
)

func Main() {
	dbPath := ".ard-bench-sql.db"
	if fs.Exists(dbPath) {
		_ = fs.Delete(dbPath).Expect("remove previous benchmark db")
	}
	db := sql.Open(dbPath).Expect("open benchmark db")
	_ = db.Exec("CREATE TABLE items(name TEXT, qty INTEGER, price INTEGER, region TEXT)").Expect("create benchmark table")
	tx := db.Begin().Expect("begin benchmark transaction")
	insert := tx.Query("INSERT INTO items(name, qty, price, region) VALUES (@name, @qty, @price, @region)")
	for i := 0; i <= 5000; i++ {
		region := "north"
		if (i % 3) == 1 {
			region = "south"
		} else {
			if (i % 3) == 2 {
				region = "west"
			}
		}
		_ = insert.Run(map[string]any{"name": ("item-" + strconv.Itoa(i)), "qty": (i % (11 + 1)), "price": ((i * 7) % (19 + 3)), "region": region}).Expect("insert benchmark row")
	}
	_ = tx.Commit().Expect("commit benchmark transaction")
	noArgs := map[string]any{}
	rows := db.Query("SELECT name, qty, price, region FROM items").All(noArgs).Expect("query benchmark rows")
	nameDecoder := decode.Field[string]("name", decode.String)
	_ = nameDecoder
	qtyDecoder := decode.Field[int]("qty", decode.Int)
	_ = qtyDecoder
	priceDecoder := decode.Field[int]("price", decode.Int)
	_ = priceDecoder
	regionDecoder := decode.Field[string]("region", decode.String)
	_ = regionDecoder
	checksum := len(rows)
	for _, row := range rows {
		name := nameDecoder(row).Expect("decode name")
		qty := qtyDecoder(row).Expect("decode qty")
		price := priceDecoder(row).Expect("decode price")
		region2 := regionDecoder(row).Expect("decode region")
		checksum = (checksum + len(name))
		checksum = (checksum + (qty * price))
		checksum = (checksum + len(region2))
	}
	_ = db.Close().Expect("close benchmark db")
	_ = fs.Delete(dbPath).Expect("delete benchmark db")
	io.Print(ardgo.AsToString(checksum))
}

func main() {
	ardgo.RegisterBuiltinExterns()
	Main()
}
