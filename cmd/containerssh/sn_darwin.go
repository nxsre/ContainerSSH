package main

// #cgo LDFLAGS: -framework CoreFoundation -framework IOKit
// #include <CoreFoundation/CoreFoundation.h>
// #include <IOKit/IOKitLib.h>
//
// const char *
// getSerialNumber()
// {
//     CFMutableDictionaryRef matching = IOServiceMatching("IOPlatformExpertDevice");
//     // kIOMasterPortDefault  deprecated in macOS 12
//     // kIOMainPortDefault  available since macOS 12
//     io_service_t service = IOServiceGetMatchingService(kIOMainPortDefault, matching);
//     CFStringRef serialNumber = IORegistryEntryCreateCFProperty(service,
//         CFSTR("IOPlatformSerialNumber"), kCFAllocatorDefault, 0);
//     const char *str = CFStringGetCStringPtr(serialNumber, kCFStringEncodingUTF8);
//     IOObjectRelease(service);
//
//     return str;
// }
import "C"

func GetSerialNumber() string {
	return C.GoString(C.getSerialNumber())
}
