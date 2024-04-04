package main

import (
	"github.com/yumaojun03/dmidecode"
	"log"
)

func GetSerialNumber() string {
	dmi, err := dmidecode.New()
	if err != nil {
		log.Println(err)
		return ""
	}

	chassis, err := dmi.Chassis()
	if err == nil {
		if chassis[0].AssetTag == "" {
			return chassis[0].AssetTag
		}
	}

	system, err := dmi.System()
	if err == nil {
		if system[0].SerialNumber == "" {
			return system[0].SerialNumber
		}
	}

	return system[0].UUID
}
