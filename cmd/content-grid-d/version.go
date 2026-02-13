package main

import (
	sdkversion "github.com/cosmos/cosmos-sdk/version"

	"content-grid-chain/app"
)

const defaultVersion = "dev"

func init() {
	if sdkversion.Name == "" {
		sdkversion.Name = app.AppName
	}
	if sdkversion.AppName == "" || sdkversion.AppName == "<appd>" {
		sdkversion.AppName = "content-grid-d"
	}
	if sdkversion.Version == "" {
		sdkversion.Version = defaultVersion
	}
}
