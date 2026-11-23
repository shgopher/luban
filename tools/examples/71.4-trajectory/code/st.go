package main

import (
	"database/sql"
	"fmt"
)

func stTest() {
	db, _ := sql.Open("mysql", "root:@tcp(127.0.0.1:16306)/ch71_track")
	db.SetMaxOpenConns(1)
	show := func(tag string) {
		rows, err := db.Query("SHOW SESSION STATUS WHERE Variable_name IN ('Handler_read_key','Handler_read_next','Bytes_sent','Handler_read_rnd_next')")
		if err != nil {
			panic(err)
		}
		fmt.Print(tag, ": ")
		for rows.Next() {
			var n, v string
			_ = rows.Scan(&n, &v)
			fmt.Printf("%s=%s ", n, v)
		}
		rows.Close()
		fmt.Println()
	}
	show("before")
	rows, err := db.Query("SELECT ts FROM track_x WHERE gh6=? AND ts>=? AND ts<?", "wx4gsk", 1793510400, 1793514000)
	if err != nil {
		panic(err)
	}
	c := 0
	for rows.Next() {
		var x int64
		_ = rows.Scan(&x)
		c++
	}
	rows.Close()
	fmt.Println("returned", c)
	show("after")
	var n, v string
	_ = db.QueryRow("SHOW SESSION STATUS LIKE 'Handler_read_next'").Scan(&n, &v)
	fmt.Println("like form:", n, v)
}
