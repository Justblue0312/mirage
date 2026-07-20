package testdata

type User struct {
	ID    int64  `db:"pk,identity,type:bigserial"`
	Name  string `db:"type:text,notnull"`
	Email string `db:"type:varchar(255),unique"`
}
