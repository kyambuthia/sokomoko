package main

import (
        "fmt"
        "log"
        "os"
        "database/sql"
        "text/template"

        _ "github.com/ncruces/go-sqlite3/driver"
        _ "github.com/ncruces/go-sqlite3/embed"
)

type Product struct {
        Name string     `json:"name"`
        Price float64   `json:"price"`
        Category string `json:"category"`
}

func createTableSchema() {
        db.Exec()
}

func main() {
        // create the product table.
        createProductTableQuery := `
        CREATE TABLE products (category TEXT, itemName TEXT, price REAL);
        `

        // returns all items.
        allItemsQuery := `
        SELECT * FROM products;
        `

        db, err := sql.Open("sqlite3", "../db/t.db")
        if (err != nil) {
                log.Fatal(err)
        }
        defer db.Close()

        db.Query(createProductTableQuery)

        db.Query(allItemsQuery)

        fmt.Println("Hello, Go")

        prod := Product{"Banana Cake", 300, "cake"}

        tmpl, err := template.New("tmpl").Parse("CREATE TABLE Product {{.Name}} {{.Price}} {{.Category}} \n")
        if (err != nil) {
                log.Fatal(err)
        }

        tmpl.Execute(os.Stdout, prod)
        if (err != nil) {
                log.Fatal(err)
        }
}
