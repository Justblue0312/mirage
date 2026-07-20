package schema

// PasswordHandler handles password encryption and decryption.
type PasswordHandler struct {
	Encrypt func(tableName string, plainPassword string) (encrypted string, err error)
	Decrypt func(tableName string, encrypted string) (plain string, err error)
}

func (h *PasswordHandler) canEncrypt() bool {
	return h != nil && h.Encrypt != nil
}
