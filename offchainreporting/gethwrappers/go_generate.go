// Package gethwrappers keeps track of the golang wrappers of the solidity contracts
package gethwrappers

//go:generate ./compile.sh ../../contract/src/AccessControlledOffchainAggregator.sol
//go:generate ./compile.sh ../../contract/src/OffchainAggregator.sol
//go:generate ./compile.sh ../../contract/src/ExposedOffchainAggregator.sol

//go:generate ./compile.sh ../../contract/src/TestOffchainAggregator.sol
//go:generate ./compile.sh ../../contract/src/TestValidator.sol
//go:generate ./compile.sh ../../contract/src/AccessControlTestHelper.sol
