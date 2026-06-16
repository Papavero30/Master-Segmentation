package migration

import (
	"database/sql"
	"fmt"
	"log"
)

type MigrationManager struct {
	DB *sql.DB
}

type Migration struct {
	Name    string
	UpSQL   string
	DownSQL string
}

func NewMigrationManager(db *sql.DB) *MigrationManager {
	return &MigrationManager{
		DB: db,
	}
}

func (m *MigrationManager) MigrateUp() error {

	var tableExists bool
	err := m.DB.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'migrations'
		)
	`).Scan(&tableExists)

	if err != nil {
		return fmt.Errorf("failed to check if migrations table exists: %w", err)
	}

	if !tableExists {
		log.Println("Creating migrations table...")
		_, err := m.DB.Exec(`
			CREATE TABLE migrations (
				id SERIAL PRIMARY KEY,
				name VARCHAR(255) NOT NULL,
				applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create migrations table: %w", err)
		}
	}

	log.Println("Checking for pending migrations...")

	rows, err := m.DB.Query("SELECT name FROM migrations")
	if err != nil {
		return fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("failed to scan migration row: %w", err)
		}
		applied[name] = true
	}

	migrations := GetMigrations()

	pendingCount := 0
	for _, migration := range migrations {
		if !applied[migration.Name] {
			pendingCount++
		}
	}

	if pendingCount == 0 {
		log.Println("No pending migrations to apply")
		return nil
	}

	log.Printf("Found %d pending migrations to apply", pendingCount)

	for _, migration := range migrations {
		if !applied[migration.Name] {

			log.Printf("Applying migration: %s", migration.Name)

			tx, err := m.DB.Begin()
			if err != nil {
				return fmt.Errorf("failed to begin transaction: %w", err)
			}

			if _, err := tx.Exec(migration.UpSQL); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to execute migration %s: %w", migration.Name, err)
			}

			if _, err := tx.Exec("INSERT INTO migrations (name) VALUES ($1)", migration.Name); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to record migration %s: %w", migration.Name, err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("failed to commit transaction: %w", err)
			}

			log.Printf("Successfully applied migration: %s", migration.Name)
		}
	}

	return nil
}

func (m *MigrationManager) MigrateDown() error {

	var lastMigrationName string
	err := m.DB.QueryRow(`
		SELECT name FROM migrations
		ORDER BY applied_at DESC
		LIMIT 1
	`).Scan(&lastMigrationName)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Println("No migrations to roll back")
			return nil
		}
		return fmt.Errorf("failed to get last migration: %w", err)
	}

	var downSQL string
	migrations := GetMigrations()
	for _, migration := range migrations {
		if migration.Name == lastMigrationName {
			downSQL = migration.DownSQL
			break
		}
	}

	if downSQL == "" {
		return fmt.Errorf("couldn't find down migration for %s", lastMigrationName)
	}

	tx, err := m.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if _, err := tx.Exec(downSQL); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to execute down migration %s: %w", lastMigrationName, err)
	}

	if _, err := tx.Exec("DELETE FROM migrations WHERE name = $1", lastMigrationName); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete migration record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Rolled back migration: %s", lastMigrationName)
	return nil
}

func GetMigrations() []Migration {
	return []Migration{
		{
			Name: "001_init_schema",
			UpSQL: `-- Initialize schema for patient data
CREATE SEQUENCE IF NOT EXISTS patientdata_id_seq START 1;

CREATE TABLE IF NOT EXISTS patient_data (
	id INTEGER PRIMARY KEY DEFAULT nextval('patientdata_id_seq'),
	name TEXT NOT NULL,
	data_count INTEGER DEFAULT 5
);

CREATE TABLE IF NOT EXISTS patient_data_item (
	id SERIAL PRIMARY KEY,
	address TEXT NOT NULL,
	doctor_name TEXT NOT NULL,
	files TEXT[] NOT NULL,
	gender TEXT NOT NULL,
	hospital_name TEXT NOT NULL,
	name TEXT NOT NULL,
	phone TEXT NOT NULL,
	patient_id INTEGER REFERENCES patient_data(id) ON DELETE CASCADE
);

-- Set sequence value
SELECT setval('patientdata_id_seq', COALESCE((SELECT MAX(id) FROM patient_data), 0) + 1, false);`,
			DownSQL: `-- Drop tables in reverse order
DROP TABLE IF EXISTS patient_data_item;
DROP TABLE IF EXISTS patient_data;
DROP SEQUENCE IF EXISTS patientdata_id_seq;`,
		},
		// Compression metadata additions to scans table
		{
			Name: "002_add_scans_compression_columns",
			UpSQL: `ALTER TABLE scans ADD COLUMN IF NOT EXISTS original_size_bytes BIGINT;
ALTER TABLE scans ADD COLUMN IF NOT EXISTS compressed_size_bytes BIGINT;
ALTER TABLE scans ADD COLUMN IF NOT EXISTS compression_ratio DOUBLE PRECISION;
ALTER TABLE scans ADD COLUMN IF NOT EXISTS last_decompressed_at TIMESTAMPTZ;
ALTER TABLE scans ADD COLUMN IF NOT EXISTS is_decompressed_cached BOOLEAN DEFAULT FALSE;`,
			DownSQL: `ALTER TABLE scans DROP COLUMN IF EXISTS original_size_bytes;
ALTER TABLE scans DROP COLUMN IF EXISTS compressed_size_bytes;
ALTER TABLE scans DROP COLUMN IF EXISTS compression_ratio;
ALTER TABLE scans DROP COLUMN IF EXISTS last_decompressed_at;
ALTER TABLE scans DROP COLUMN IF EXISTS is_decompressed_cached;`,
		},
		// Ingestion & manifest pipeline columns
		{
			Name: "003_add_scans_ingestion_columns",
			UpSQL: `ALTER TABLE scans ADD COLUMN IF NOT EXISTS upload_status VARCHAR(50) DEFAULT 'pending';
ALTER TABLE scans ADD COLUMN IF NOT EXISTS expected_file_count INTEGER;
ALTER TABLE scans ADD COLUMN IF NOT EXISTS received_file_count INTEGER DEFAULT 0;
ALTER TABLE scans ADD COLUMN IF NOT EXISTS manifest_checksum VARCHAR(128);
ALTER TABLE scans ADD COLUMN IF NOT EXISTS compressed_manifest_checksum VARCHAR(128);
ALTER TABLE scans ADD COLUMN IF NOT EXISTS last_error TEXT;
ALTER TABLE scans ADD COLUMN IF NOT EXISTS cache_expires_at TIMESTAMPTZ;`,
			DownSQL: `ALTER TABLE scans DROP COLUMN IF EXISTS upload_status;
ALTER TABLE scans DROP COLUMN IF EXISTS expected_file_count;
ALTER TABLE scans DROP COLUMN IF EXISTS received_file_count;
ALTER TABLE scans DROP COLUMN IF EXISTS manifest_checksum;
ALTER TABLE scans DROP COLUMN IF EXISTS compressed_manifest_checksum;
ALTER TABLE scans DROP COLUMN IF EXISTS last_error;
ALTER TABLE scans DROP COLUMN IF EXISTS cache_expires_at;`,
		},
		// Job log table for background jobs
		{
			Name: "004_add_job_logs_table",
			UpSQL: `CREATE TABLE IF NOT EXISTS job_logs (
				id SERIAL PRIMARY KEY,
				job_id VARCHAR(255) NOT NULL,
				sid VARCHAR(255) NOT NULL,
				type VARCHAR(100) NOT NULL,
				status VARCHAR(50) NOT NULL,
				attempts INTEGER NOT NULL DEFAULT 0,
				max_retry INTEGER NOT NULL DEFAULT 0,
				enqueued_at TIMESTAMPTZ NOT NULL,
				started_at TIMESTAMPTZ,
				finished_at TIMESTAMPTZ,
				last_error TEXT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_job_logs_sid ON job_logs(sid);
			CREATE INDEX IF NOT EXISTS idx_job_logs_job_id ON job_logs(job_id);`,
			DownSQL: `DROP TABLE IF EXISTS job_logs;`,
		},
		// Scan files table for unique per-file tracking
		{
			Name: "005_add_scan_files_table",
			UpSQL: `CREATE TABLE IF NOT EXISTS scan_files (
				id SERIAL PRIMARY KEY,
				sid VARCHAR(255) NOT NULL,
				rel_path TEXT NOT NULL,
				size_bytes BIGINT,
				checksum VARCHAR(128),
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE (sid, rel_path)
			);`,
			DownSQL: `DROP TABLE IF EXISTS scan_files;`,
		},
		// Performance & Security: Add indexes and foreign key constraints
		{
			Name: "006_add_performance_indexes_and_constraints",
			UpSQL: `
			-- ========================================
			-- PROFILES TABLE INDEXES
			-- ========================================
			-- first_uid is already UNIQUE in GORM entity, ensure index exists
			CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_first_uid ON profiles(first_uid);
			
			-- Index on deleted_at for soft delete queries (GORM feature)
			CREATE INDEX IF NOT EXISTS idx_profiles_deleted_at ON profiles(deleted_at);
			
			
			-- ========================================
			-- DEVICES TABLE INDEXES
			-- ========================================
			-- fingerprint is already UNIQUE in GORM entity, ensure index exists
			CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_fingerprint ON devices(fingerprint);
			
			-- Index on is_active for active device queries
			CREATE INDEX IF NOT EXISTS idx_devices_is_active ON devices(is_active);
			
			-- Index on last_access_time for session management
			CREATE INDEX IF NOT EXISTS idx_devices_last_access_time ON devices(last_access_time);
			
			
			-- ========================================
			-- SCANS TABLE INDEXES
			-- ========================================
			-- Index on first_uid (foreign key to profiles)
			CREATE INDEX IF NOT EXISTS idx_scans_first_uid ON scans(first_uid);
			
			-- Index on sid (scan identifier, frequently queried)
			CREATE INDEX IF NOT EXISTS idx_scans_sid ON scans(sid);
			
			-- Index on upload_status for filtering pending/completed scans
			CREATE INDEX IF NOT EXISTS idx_scans_upload_status ON scans(upload_status);
			
			-- Index on deleted_at for soft delete queries
			CREATE INDEX IF NOT EXISTS idx_scans_deleted_at ON scans(deleted_at);
			
			
			-- ========================================
			-- USER_ROLES TABLE INDEXES
			-- ========================================
			-- Index on device_id (foreign key to devices)
			CREATE INDEX IF NOT EXISTS idx_user_roles_device_id ON user_roles(device_id);
			
			-- Index on role for role-based queries
			CREATE INDEX IF NOT EXISTS idx_user_roles_role ON user_roles(role);
			
			
			-- ========================================
			-- SCAN_FILES TABLE INDEXES
			-- ========================================
			-- Composite index on sid for faster file lookups per scan
			CREATE INDEX IF NOT EXISTS idx_scan_files_sid ON scan_files(sid);
			
			
			-- ========================================
			-- FOREIGN KEY CONSTRAINTS (Data Integrity)
			-- ========================================
			-- Note: These use ON DELETE CASCADE for automatic cleanup
			
			-- scans.first_uid -> profiles.first_uid (scans belong to profiles)
			DO $$
			BEGIN
				IF NOT EXISTS (
					SELECT 1 FROM pg_constraint 
					WHERE conname = 'fk_scans_first_uid'
				) THEN
					ALTER TABLE scans 
					ADD CONSTRAINT fk_scans_first_uid 
					FOREIGN KEY (first_uid) 
					REFERENCES profiles(first_uid) 
					ON DELETE CASCADE;
				END IF;
			END $$;
			
			-- user_roles.device_id -> devices.id (roles belong to devices)
			DO $$
			BEGIN
				IF NOT EXISTS (
					SELECT 1 FROM pg_constraint 
					WHERE conname = 'fk_user_roles_device_id'
				) THEN
					ALTER TABLE user_roles 
					ADD CONSTRAINT fk_user_roles_device_id 
					FOREIGN KEY (device_id) 
					REFERENCES devices(id) 
					ON DELETE CASCADE;
				END IF;
			END $$;
			
			-- scan_files.sid -> scans.sid (files belong to scans)
			DO $$
			BEGIN
				IF NOT EXISTS (
					SELECT 1 FROM pg_constraint 
					WHERE conname = 'fk_scan_files_sid'
				) THEN
					ALTER TABLE scan_files 
					ADD CONSTRAINT fk_scan_files_sid 
					FOREIGN KEY (sid) 
					REFERENCES scans(sid) 
					ON DELETE CASCADE;
				END IF;
			END $$;`,
			DownSQL: `
			-- Drop foreign key constraints
			ALTER TABLE scans DROP CONSTRAINT IF EXISTS fk_scans_first_uid;
			ALTER TABLE user_roles DROP CONSTRAINT IF EXISTS fk_user_roles_device_id;
			ALTER TABLE scan_files DROP CONSTRAINT IF EXISTS fk_scan_files_sid;
			
			-- Drop indexes
			DROP INDEX IF EXISTS idx_profiles_first_uid;
			DROP INDEX IF EXISTS idx_profiles_deleted_at;
			DROP INDEX IF EXISTS idx_devices_fingerprint;
			DROP INDEX IF EXISTS idx_devices_is_active;
			DROP INDEX IF EXISTS idx_devices_last_access_time;
			DROP INDEX IF EXISTS idx_scans_first_uid;
			DROP INDEX IF EXISTS idx_scans_sid;
			DROP INDEX IF EXISTS idx_scans_upload_status;
			DROP INDEX IF EXISTS idx_scans_deleted_at;
			DROP INDEX IF EXISTS idx_user_roles_device_id;
			DROP INDEX IF EXISTS idx_user_roles_role;
			DROP INDEX IF EXISTS idx_scan_files_sid;`,
		},
		{
			Name: "007_add_segmentation_experiments",
			UpSQL: `CREATE TABLE IF NOT EXISTS segmentation_experiments (
				id SERIAL PRIMARY KEY,
				task_id VARCHAR(64) UNIQUE NOT NULL,
				scan_sid VARCHAR(64) NOT NULL,
				slice_index INTEGER NOT NULL,
				cluster_mode VARCHAR(20) NOT NULL,
				chunk_size INTEGER NOT NULL,
				overlap_size INTEGER NOT NULL,
				total_chunks INTEGER NOT NULL,
				submitted_at TIMESTAMPTZ NOT NULL,
				completed_at TIMESTAMPTZ,
				total_latency_ms INTEGER,
				queue_wait_ms INTEGER,
				inference_ms INTEGER,
				merge_ms INTEGER,
				dice_score DOUBLE PRECISION,
				hausdorff_distance DOUBLE PRECISION,
				status VARCHAR(20) NOT NULL,
				error_message TEXT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
			);

			CREATE INDEX IF NOT EXISTS idx_seg_exp_cluster_mode ON segmentation_experiments(cluster_mode);
			CREATE INDEX IF NOT EXISTS idx_seg_exp_submitted_at ON segmentation_experiments(submitted_at);

			CREATE TABLE IF NOT EXISTS segmentation_chunk_logs (
				id SERIAL PRIMARY KEY,
				task_id VARCHAR(64) NOT NULL,
				chunk_id INTEGER NOT NULL,
				worker_name VARCHAR(128) NOT NULL,
				gpu_type VARCHAR(64),
				cluster_mode VARCHAR(20),
				queue_wait_ms INTEGER,
				inference_ms INTEGER,
				total_chunk_ms INTEGER,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE (task_id, chunk_id),
				FOREIGN KEY (task_id) REFERENCES segmentation_experiments(task_id) ON DELETE CASCADE
			);

			CREATE INDEX IF NOT EXISTS idx_chunk_logs_task_id ON segmentation_chunk_logs(task_id);
			CREATE INDEX IF NOT EXISTS idx_chunk_logs_worker ON segmentation_chunk_logs(worker_name);
			CREATE INDEX IF NOT EXISTS idx_chunk_logs_cluster_mode ON segmentation_chunk_logs(cluster_mode);
			CREATE INDEX IF NOT EXISTS idx_chunk_logs_gpu_type ON segmentation_chunk_logs(gpu_type);`,
			DownSQL: `DROP TABLE IF EXISTS segmentation_chunk_logs;
			DROP TABLE IF EXISTS segmentation_experiments;
			DROP INDEX IF EXISTS idx_seg_exp_cluster_mode;
			DROP INDEX IF EXISTS idx_seg_exp_submitted_at;
			DROP INDEX IF EXISTS idx_chunk_logs_task_id;
			DROP INDEX IF EXISTS idx_chunk_logs_worker;
			DROP INDEX IF EXISTS idx_chunk_logs_cluster_mode;
			DROP INDEX IF EXISTS idx_chunk_logs_gpu_type;`,
		},
	}
}

func AddMigration(name string, upSQL string, downSQL string) {

	log.Printf("Migration %s registered (stub function, not actually storing)", name)
}
