package main

import (
	"fmt"
	"log"
	"os"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/config"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/migration"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/seeder"
)

func main() {
	
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("Available commands:")
		fmt.Println("  go run .cmd.go reset - Reset and seed the database")
		fmt.Println("  go run .cmd.go migrate - Run pending migrations")
		fmt.Println("  go run .cmd.go rollback - Rollback the latest migration")
		fmt.Println("  go run .cmd.go generate-migration - Generate new migration files")
		fmt.Println("  go run .cmd.go purge - Remove all data without dropping tables")
		return
	}

	
	cfg := config.LoadConfig()

	
	db, err := seeder.InitDB(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	fmt.Println("Connected to database successfully")

	command := args[0]

	switch command {
	case "reset":
		
		fmt.Println("\n⚠️  COMPLETE DATABASE RESET ⚠️")
		fmt.Println("This will drop ALL tables, sequences and data in the database.")
		fmt.Println("\nStarting reset process...")

		
		if err := seeder.InitializeDB(db); err != nil {
			log.Fatalf("❌ Database reset failed: %v", err)
		}

		
		var patientCount, itemCount int
		err = db.QueryRow("SELECT COUNT(*) FROM patient_data").Scan(&patientCount)
		if err != nil {
			log.Fatalf("❌ Error verifying reset: %v", err)
		}

		err = db.QueryRow("SELECT COUNT(*) FROM patient_data_item").Scan(&itemCount)
		if err != nil {
			log.Fatalf("❌ Error verifying reset: %v", err)
		}

		fmt.Println("\n✅ Database reset successful!")
		fmt.Printf("→ %d patients with %d data items created\n", patientCount, itemCount)
		fmt.Println("\nNote: The migrations table has been dropped - you will need to rerun migrations if needed")

	case "migrate":
		
		fmt.Println("Running migrations...")
		migrationManager := migration.NewMigrationManager(db)
		if err := migrationManager.MigrateUp(); err != nil {
			log.Fatalf("Failed to run migrations: %v", err)
		}
		fmt.Println("Migrations applied successfully")

	case "rollback":
		
		fmt.Println("Rolling back the latest migration...")
		migrationManager := migration.NewMigrationManager(db)
		if err := migrationManager.MigrateDown(); err != nil {
			log.Fatalf("Failed to roll back migration: %v", err)
		}
		fmt.Println("Migration rolled back successfully")

	case "generate-migration":
		
		fmt.Println("Migrations are now handled programmatically.")
		fmt.Println("To add a new migration, define it in the migration.GetMigrations() function.")
		fmt.Println("No SQL files are generated with this new approach.")

	case "purge":
		
		fmt.Println("\n⚠️  DATABASE PURGE ⚠️")
		fmt.Println("This will remove ALL data from the database tables without dropping the tables.")
		fmt.Println("\nStarting purge process...")

		
		if err := seeder.PurgeAllData(db); err != nil {
			log.Fatalf("❌ Database purge failed: %v", err)
		}

		
		var patientCount, itemCount int
		err = db.QueryRow("SELECT COUNT(*) FROM patient_data").Scan(&patientCount)
		if err != nil {
			log.Fatalf("❌ Error verifying purge: %v", err)
		}

		err = db.QueryRow("SELECT COUNT(*) FROM patient_data_item").Scan(&itemCount)
		if err != nil {
			log.Fatalf("❌ Error verifying purge: %v", err)
		}

		fmt.Println("\n✅ Database purge successful!")
		fmt.Printf("→ Patient table now has %d records\n", patientCount)
		fmt.Printf("→ Patient data item table now has %d records\n", itemCount)
		fmt.Println("\nNote: Table structures have been preserved.")

	default:
		fmt.Println("Unknown command:", command)
		fmt.Println("Available commands: reset, migrate, rollback, generate-migration, purge")
	}
}
