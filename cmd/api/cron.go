package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

func startCronJob(db *sql.DB) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		refreshMaterializedViews(db)
	}
}

func refreshMaterializedViews(db *sql.DB) {
	views := []string{
		"klines_1m",
		"klines_1h",
		"klines_1w",
	}

	for _, view := range views {
		query := fmt.Sprintf("REFRESH MATERIALIZED VIEW %s", view)
		if _, err := db.Exec(query); err != nil {
			log.Printf("Error refreshing view %s: %v", view, err)
		}
	}

	// log.Println("Materialized views refreshed successfully")
}
