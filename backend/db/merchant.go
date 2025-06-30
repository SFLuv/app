package db

import (
	"gorm.io/gorm"
)

func MerchantDB() *SFLuvDB {
	// Initialize the database connection for merchants
	db := InitDB("merchants")

	err := db.db.AutoMigrate(&Merchant{}, &Person{}, &Address{}, &Category{})
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
