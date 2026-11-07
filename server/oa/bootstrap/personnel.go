package bootstrap

import "gorm.io/gorm"

func backfillDepartments(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Preserve the display text and version of every existing employee.
		if err := tx.Exec(`INSERT INTO oa_departments (name, version)
			SELECT DISTINCT TRIM(department), 1 FROM oa_employees
			WHERE department_id IS NULL AND TRIM(department) <> ''
			ON DUPLICATE KEY UPDATE id = oa_departments.id`).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE oa_employees AS e JOIN oa_departments AS d ON d.name = TRIM(e.department)
			SET e.department_id = d.id WHERE e.department_id IS NULL`).Error
	})
}
