package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Mikem03/Local-mail/internal/config"
	"github.com/Mikem03/Local-mail/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	migrationDir := flag.String("dir", "migrations", "directory containing ordered SQL migration files")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, err := store.Open(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.Ping(ctx); err != nil {
		return err
	}
	if err := db.ApplyMigrations(ctx, *migrationDir); err != nil {
		return err
	}
	fmt.Println("Database migrations are up to date.")
	return nil
}
