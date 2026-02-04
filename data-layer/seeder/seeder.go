package seeder

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/data-layer/config"
	_ "github.com/lib/pq"
)

func InitDB(cfg config.Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.PostgresConnectionString())
	if err != nil {
		log.Printf("Error opening database: %v", err)
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		log.Printf("Error connecting to database: %v", err)
		return nil, err
	}

	fmt.Println("Successfully connected to database")
	return db, nil
}

func CreateTables(db *sql.DB) error {
	log.Println("Starting database reset process...")

	
	var tableCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&tableCount)
	if err != nil {
		return fmt.Errorf("failed to count tables: %w", err)
	}
	log.Printf("Found %d tables in database before reset", tableCount)

	
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	
	_, err = tx.Exec(`SET CONSTRAINTS ALL DEFERRED;`)
	if err != nil {
		tx.Rollback()
		log.Printf("Warning: Could not defer constraints: %v", err)
	}

	
	log.Println("Step 1: Dropping all tables in dependency order...")

	
	log.Println("Dropping patient_data_item table...")
	_, err = tx.Exec(`DROP TABLE IF EXISTS patient_data_item;`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to drop patient_data_item: %w", err)
	}

	
	log.Println("Dropping patient_data table...")
	_, err = tx.Exec(`DROP TABLE IF EXISTS patient_data;`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to drop patient_data: %w", err)
	}

	
	log.Println("Dropping migrations table...")
	_, err = tx.Exec(`DROP TABLE IF EXISTS migrations;`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to drop migrations: %w", err)
	}

	
	log.Println("Dropping patientdata_id_seq sequence...")
	_, err = tx.Exec(`DROP SEQUENCE IF EXISTS patientdata_id_seq;`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to drop sequence: %w", err)
	}

	
	var remainingTables int
	err = tx.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&remainingTables)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to verify table drops: %w", err)
	}
	log.Printf("Remaining tables after drops: %d", remainingTables)

	
	log.Println("Step 2: Creating sequence...")
	_, err = tx.Exec(`CREATE SEQUENCE patientdata_id_seq START 1;`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create sequence: %w", err)
	}

	
	log.Println("Step 3: Creating patient_data table...")
	patientDataTable := `
    CREATE TABLE patient_data (
        id INTEGER PRIMARY KEY DEFAULT nextval('patientdata_id_seq'),
        name TEXT NOT NULL,
        data_count INTEGER DEFAULT 5
    );`

	_, err = tx.Exec(patientDataTable)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create patient_data table: %w", err)
	}

	
	log.Println("Step 4: Creating patient_data_item table...")
	patientDataItemTable := `
    CREATE TABLE patient_data_item (
        id SERIAL PRIMARY KEY,
        address TEXT NOT NULL,
        doctor_name TEXT NOT NULL,
        files TEXT[] NOT NULL,
        gender TEXT NOT NULL,
        hospital_name TEXT NOT NULL,
        name TEXT NOT NULL,
        phone TEXT NOT NULL,
        patient_id INTEGER REFERENCES patient_data(id) ON DELETE CASCADE
    );`

	_, err = tx.Exec(patientDataItemTable)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create patient_data_item table: %w", err)
	}

	
	log.Println("Step 5: Resetting sequence values...")
	_, err = tx.Exec(`SELECT setval('patientdata_id_seq', 1, false);`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to reset sequence: %w", err)
	}

	
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	
	err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&tableCount)
	if err != nil {
		return fmt.Errorf("failed to verify final table count: %w", err)
	}
	log.Printf("Reset complete. Database now has %d tables.", tableCount)

	return nil
}

func SeedDummyData(db *sql.DB) error {
	log.Println("Starting to seed dummy data...")

	
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for seeding: %w", err)
	}

	
	var patientCount int
	err = tx.QueryRow("SELECT COUNT(*) FROM patient_data").Scan(&patientCount)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to check existing patient data: %w", err)
	}

	if patientCount > 0 {
		log.Printf("Warning: Found %d existing patients. Proceeding with additional seeding.", patientCount)
	}

	
	log.Println("Inserting patient data...")
	patientInsert := `
    INSERT INTO patient_data (name, data_count) VALUES
    ('John Doe', 2),
    ('Jane Smith', 1),
    ('Robert Johnson', 3)
    RETURNING id;`

	rows, err := tx.Query(patientInsert)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to insert patient data: %w", err)
	}

	
	rows.Close()

	
	log.Println("Inserting patient data items...")
	itemInsert := `
    INSERT INTO patient_data_item (address, doctor_name, files, gender, hospital_name, name, phone, patient_id)
    SELECT '123 Main St', 'Dr. Wilson', ARRAY['scan1.jpg', 'report1.pdf'], 'Male', 'Central Hospital', 'John Doe', '555-1234', id
    FROM patient_data WHERE name = 'John Doe' LIMIT 1;

    INSERT INTO patient_data_item (address, doctor_name, files, gender, hospital_name, name, phone, patient_id)
    SELECT '456 Oak St', 'Dr. Roberts', ARRAY['scan2.jpg', 'report2.pdf'], 'Male', 'Central Hospital', 'John Doe', '555-1234', id
    FROM patient_data WHERE name = 'John Doe' LIMIT 1;

    INSERT INTO patient_data_item (address, doctor_name, files, gender, hospital_name, name, phone, patient_id)
    SELECT '789 Pine St', 'Dr. Davis', ARRAY['scan3.jpg'], 'Female', 'West Hospital', 'Jane Smith', '555-5678', id
    FROM patient_data WHERE name = 'Jane Smith' LIMIT 1;

    INSERT INTO patient_data_item (address, doctor_name, files, gender, hospital_name, name, phone, patient_id)
    SELECT '101 Elm St', 'Dr. Brown', ARRAY['scan4.jpg', 'report4.pdf', 'notes1.txt'], 'Male', 'East Hospital', 'Robert Johnson', '555-9012', id
    FROM patient_data WHERE name = 'Robert Johnson' LIMIT 1;

    INSERT INTO patient_data_item (address, doctor_name, files, gender, hospital_name, name, phone, patient_id)
    SELECT '202 Cedar St', 'Dr. Miller', ARRAY['scan5.jpg', 'report5.pdf'], 'Male', 'East Hospital', 'Robert Johnson', '555-9012', id
    FROM patient_data WHERE name = 'Robert Johnson' LIMIT 1;

    INSERT INTO patient_data_item (address, doctor_name, files, gender, hospital_name, name, phone, patient_id)
    SELECT '303 Birch St', 'Dr. Taylor', ARRAY['scan6.jpg'], 'Male', 'East Hospital', 'Robert Johnson', '555-9012', id
    FROM patient_data WHERE name = 'Robert Johnson' LIMIT 1;`

	_, err = tx.Exec(itemInsert)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to insert patient data items: %w", err)
	}

	
	var itemCount int
	err = tx.QueryRow("SELECT COUNT(*) FROM patient_data_item").Scan(&itemCount)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to verify data seeding: %w", err)
	}

	
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit data seeding: %w", err)
	}

	log.Printf("Successfully seeded %d patient records with %d data items", 3, itemCount)
	return nil
}


func GenerateMigrationFiles() error {
	
	
	log.Println("Migration files are now handled programmatically - no files generated")
	return nil
}


func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}

	
	for {
		if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "."
}


func InitializeDB(db *sql.DB) error {
	
	if err := CreateTables(db); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	
	if err := SeedDummyData(db); err != nil {
		return fmt.Errorf("failed to seed data: %w", err)
	}

	return nil
}


func PurgeAllData(db *sql.DB) error {
	log.Println("Starting database purge process...")

	
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	
	log.Println("Step 1: Deleting all data from tables in dependency order...")

	
	log.Println("Purging patient_data_item table...")
	result, err := tx.Exec(`DELETE FROM patient_data_item;`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to purge patient_data_item: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	log.Printf("Deleted %d rows from patient_data_item", rowsAffected)

	
	log.Println("Purging patient_data table...")
	result, err = tx.Exec(`DELETE FROM patient_data;`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to purge patient_data: %w", err)
	}
	rowsAffected, _ = result.RowsAffected()
	log.Printf("Deleted %d rows from patient_data", rowsAffected)

	
	log.Println("Step 2: Resetting sequence values...")
	_, err = tx.Exec(`ALTER SEQUENCE patientdata_id_seq RESTART WITH 1;`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to reset patient_data sequence: %w", err)
	}

	_, err = tx.Exec(`ALTER SEQUENCE patient_data_item_id_seq RESTART WITH 1;`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to reset patient_data_item sequence: %w", err)
	}

	
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Println("Database purge completed successfully.")
	return nil
}
