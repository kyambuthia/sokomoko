package db

import "database/sql"

// CreateUser inserts a new user into the database
func (s *Store) CreateUser(user User) (int64, error) {
	if user.Slug == "" {
		user.Slug = user.Username
	}

	stmt, err := s.DB.Prepare(
		"INSERT INTO users (username, email, password_hash, salt, role, slug) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(user.Username, user.Email, user.PasswordHash, user.Salt, user.Role, user.Slug)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

// GetUserByUsername retrieves a user by their username
func (s *Store) GetUserByUsername(username string) (*User, error) {
	row := s.DB.QueryRow(
		"SELECT id, username, email, password_hash, salt, role, slug, created_at, updated_at, deleted_at FROM users WHERE username = ? AND deleted_at IS NULL", username)

	user := &User{}
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Salt,
		&user.Role,
		&user.Slug,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByID retrieves a user by their ID
func (s *Store) GetUserByID(id int) (*User, error) {
	row := s.DB.QueryRow(
		"SELECT id, username, email, password_hash, salt, role, slug, created_at, updated_at, deleted_at FROM users WHERE id = ? AND deleted_at IS NULL", id)

	user := &User{}
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Salt,
		&user.Role,
		&user.Slug,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Store) GetUserByEmail(email string) (*User, error) {
	row := s.DB.QueryRow(
		"SELECT id, username, email, password_hash, salt, role, slug, created_at, updated_at, deleted_at FROM users WHERE email = ? AND deleted_at IS NULL", email)

	user := &User{}
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Salt,
		&user.Role,
		&user.Slug,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Store) CountUsersByRole(role string) (int, error) {
	row := s.DB.QueryRow("SELECT COUNT(*) FROM users WHERE role = ? AND deleted_at IS NULL", role)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) HasAdminUser() (bool, error) {
	count, err := s.CountUsersByRole("admin")
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) UpdateUserPassword(userID int, passwordHash, salt string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.Exec(
		"UPDATE users SET password_hash = ?, salt = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND deleted_at IS NULL",
		passwordHash, salt, userID,
	)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return tx.Commit()
	}

	if _, err = tx.Exec("DELETE FROM sessions WHERE user_id = ?", userID); err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateUser updates an existing user's information
func (s *Store) UpdateUser(user User) error {
	stmt, err := s.DB.Prepare(
		"UPDATE users SET username = ?, email = ?, password_hash = ?, salt = ?, role = ?, slug = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(user.Username, user.Email, user.PasswordHash, user.Salt, user.Role, user.Slug, user.ID)
	if err != nil {
		return err
	}
	return nil
}

// DeleteUser deletes a user from the database by ID
func (s *Store) DeleteUser(id int) error {
	stmt, err := s.DB.Prepare("UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(id)
	if err != nil {
		return err
	}
	return nil
}
