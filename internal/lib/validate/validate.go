package validate

import "net/mail"

func Email(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email
}
