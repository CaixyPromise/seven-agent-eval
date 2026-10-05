package core

import "database/sql"

// Projection databases use one connection and serialize writers. SQLite can
// leave its transaction open when COMMIT returns SQLITE_BUSY, while database/sql
// marks the Tx done. Release that underlying transaction before journal replay;
// otherwise every following Begin fails and acknowledged evidence stays stuck.
func commitProjection(db *sql.DB, tx *sql.Tx) error {
	err := tx.Commit()
	if err != nil {
		_, _ = db.Exec("ROLLBACK")
	}
	return err
}
