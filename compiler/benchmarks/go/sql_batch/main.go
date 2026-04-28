package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	dbPath := ".ard-bench-sql.db"
	if _, err := os.Stat(dbPath); err == nil {
		must(os.Remove(dbPath))
	} else if !os.IsNotExist(err) {
		panic(err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	must(err)
	defer db.Close()

	_, err = db.Exec("CREATE TABLE items(name TEXT, qty INTEGER, price INTEGER, region TEXT)")
	must(err)

	tx, err := db.Begin()
	must(err)
	insert, err := tx.Prepare("INSERT INTO items(name, qty, price, region) VALUES (?, ?, ?, ?)")
	must(err)
	for i := 0; i <= 5000; i++ {
		region := "north"
		switch i % 3 {
		case 1:
			region = "south"
		case 2:
			region = "west"
		}

		_, err := insert.Exec(fmt.Sprintf("item-%d", i), i%11+1, i*7%19+3, region)
		must(err)
	}
	must(insert.Close())
	must(tx.Commit())

	rows, err := db.Query("SELECT name, qty, price, region FROM items")
	must(err)
	defer rows.Close()

	checksum := 0
	for rows.Next() {
		var name string
		var qty int
		var price int
		var region string
		must(rows.Scan(&name, &qty, &price, &region))

		checksum++
		checksum += len(name)
		checksum += qty * price
		checksum += len(region)
	}
	must(rows.Err())

	must(db.Close())
	must(os.Remove(dbPath))
	fmt.Print(checksum)
}
