package validate

import "net/mail"

// Email - проверяет, что строка является голым адресом вида user@example.com
// (без имени и угловых скобок, которые тоже допускает mail.ParseAddress)
func Email(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email
}
