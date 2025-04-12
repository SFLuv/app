package db

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func MerchantDB() *gorm.DB {
	dbPath := DBPath("merchant")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if db == nil {
		return nil
	}

	err = db.AutoMigrate(&Merchant{}, &Person{}, &Address{}, &Category{})
	if err != nil {
		panic(err)
	}

	return db
}

type Merchant struct {
	gorm.Model
	Address     Address
	AddressID   uint
	Email       string
	Name        string
	Description string
	Website     string
	Phone       string
	Category    Category
	CategoryID  uint
	Contact     Person
	ContactID   uint
}

type Person struct {
	gorm.Model
	FirstName string
	LastName  string
	Email     string
	Phone     string
}

type Address struct {
	gorm.Model
	Street   string
	Street_2 string
	City     string
	State    string
	Zip      string
}

type Category struct {
	gorm.Model
	Name string
}
