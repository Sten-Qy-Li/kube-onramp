package platform

import (
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func detect() Info {
	v := windows.RtlGetVersion()
	info := Info{
		Version: fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber),
		Build:   int(v.BuildNumber),
	}
	var productName, displayVersion string
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE); err == nil {
		productName, _, _ = k.GetStringValue("ProductName")
		displayVersion, _, _ = k.GetStringValue("DisplayVersion")
		if displayVersion == "" {
			displayVersion, _, _ = k.GetStringValue("ReleaseId")
		}
		_ = k.Close()
	}
	info.Name = WindowsName(productName, displayVersion, info.Build, runtime.GOARCH)
	info.Shell = "PowerShell unknown"
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\PowerShell\3\PowerShellEngine`, registry.QUERY_VALUE); err == nil {
		if ver, _, err := k.GetStringValue("PowerShellVersion"); err == nil {
			parts := strings.SplitN(ver, ".", 3)
			if len(parts) >= 2 {
				info.Shell = "PowerShell " + parts[0] + "." + parts[1]
			}
		}
		_ = k.Close()
	}
	return info
}
