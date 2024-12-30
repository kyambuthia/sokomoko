package main

import (
        "fmt"
        "log"
        "database/sql"

        _ "github.com/ncruces/go-sqlite3/driver"
        _ "github.com/ncruces/go-sqlite3/embed"
)

type Product struct {
        name string     `json:"name"`
        price float64   `json:"price"`
        category string `json:"category"`
}

func buildDBTables() {
        fmt.Println("-- initialising the DB Tables --")

}

func main() {
        db, err := sql.Open("sqlite3", "./db/t.db")
        if (err != nil) {
                log.Fatal(err)
        }
        defer db.Close()

        var prod = Product{"Banana Cake", 300, "cake"}
        fmt.Println(prod)

        // Create DB


        err = db.QueryRow(`SELECT * FROM names`).Scan(&result, &phonenumber)
        if err != nil {
                log.Fatal(err)
        }
}
