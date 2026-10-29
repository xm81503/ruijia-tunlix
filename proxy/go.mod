module proxy

go 1.20

require client v0.0.0

require (
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/stretchr/testify v1.8.4 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

replace client => ../client
