module server

go 1.20

require client v0.0.0

replace client => ../client

require (
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/stretchr/testify v1.8.4 // indirect
)
