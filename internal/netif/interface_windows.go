package netif

import (
	"fmt"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wlanapiDLL                     = windows.NewLazySystemDLL("wlanapi.dll")
	iphlpapiDLL                    = windows.NewLazySystemDLL("iphlpapi.dll")
	procWlanOpenHandle             = wlanapiDLL.NewProc("WlanOpenHandle")
	procWlanCloseHandle            = wlanapiDLL.NewProc("WlanCloseHandle")
	procWlanEnumInterfaces         = wlanapiDLL.NewProc("WlanEnumInterfaces")
	procWlanQueryInterface         = wlanapiDLL.NewProc("WlanQueryInterface")
	procWlanFreeMemory             = wlanapiDLL.NewProc("WlanFreeMemory")
	procConvertInterfaceGuidToLuid = iphlpapiDLL.NewProc("ConvertInterfaceGuidToLuid")
)

const (
	wlanAPIVersion2                 = 2
	wlanMaxNameLength               = 256
	dot11SSIDMaxLength              = 32
	wlanInterfaceStateConnected     = 1
	wlanIntfOpcodeCurrentConnection = 7
)

type adapterDetails struct {
	luid         uint64
	ifType       uint32
	friendlyName string
	description  string
	gateway      string
}

type wlanInterfaceInfo struct {
	InterfaceGUID windows.GUID
	Description   [wlanMaxNameLength]uint16
	State         uint32
}

type wlanInterfaceInfoList struct {
	NumberOfItems uint32
	Index         uint32
	InterfaceInfo [1]wlanInterfaceInfo
}

type dot11SSID struct {
	Length uint32
	SSID   [dot11SSIDMaxLength]byte
}

type wlanAssociationAttributes struct {
	SSID          dot11SSID
	BSSType       uint32
	BSSID         [6]byte
	PhyType       uint32
	PhyIndex      uint32
	SignalQuality uint32
	RxRate        uint32
	TxRate        uint32
}

type wlanSecurityAttributes struct {
	SecurityEnabled int32
	OneXEnabled     int32
	AuthAlgorithm   uint32
	CipherAlgorithm uint32
}

type wlanConnectionAttributes struct {
	State    uint32
	Mode     uint32
	Profile  [wlanMaxNameLength]uint16
	Assoc    wlanAssociationAttributes
	Security wlanSecurityAttributes
}

// populateDetails fills in friendly names, gateways, and LUIDs using GetAdaptersAddresses.
func populateDetails(interfaces []*NetInterface) {
	detailsByIP, err := getAdapterDetailsByIP()
	if err != nil {
		return
	}

	for _, ni := range interfaces {
		details, ok := detailsByIP[ni.IP.String()]
		if !ok {
			continue
		}
		ni.FriendlyName = firstNonEmpty(details.friendlyName, ni.Name)
		ni.Gateway = details.gateway
		ni.Luid = details.luid
	}
}

// RefreshNetworkNames updates each interface's connected network name.
func RefreshNetworkNames(interfaces []*NetInterface) {
	detailsByIP, err := getAdapterDetailsByIP()
	if err != nil {
		return
	}

	wifiSSIDs, err := getWiFiSSIDsByLUID()
	if err != nil {
		wifiSSIDs = map[uint64]string{}
	}

	for _, ni := range interfaces {
		details, ok := detailsByIP[ni.IP.String()]
		if !ok {
			ni.SetNetworkName("")
			continue
		}
		ni.SetNetworkName(buildNetworkName(details, wifiSSIDs[details.luid]))
	}
}

// GetFriendlyName returns a friendly display name for the interface.
func GetFriendlyName(ni *NetInterface) string {
	if ni.FriendlyName != "" {
		return fmt.Sprintf("%s (%s)", ni.FriendlyName, ni.IP.String())
	}
	return fmt.Sprintf("%s (%s)", ni.Name, ni.IP.String())
}

func getAdapterDetailsByIP() (map[string]adapterDetails, error) {
	first, err := getAdaptersAddresses(windows.GAA_FLAG_INCLUDE_PREFIX | windows.GAA_FLAG_INCLUDE_GATEWAYS)
	if err != nil {
		return nil, err
	}
	return collectAdapterDetailsByIP(first), nil
}

func getAdaptersAddresses(flags uint32) (*windows.IpAdapterAddresses, error) {
	var size uint32 = 15 * 1024

	for {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, flags, 0, first, &size)
		if err == nil {
			return first, nil
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil, err
		}
	}
}

func collectAdapterDetailsByIP(first *windows.IpAdapterAddresses) map[string]adapterDetails {
	lookup := make(map[string]adapterDetails)

	for aa := first; aa != nil; aa = aa.Next {
		if aa.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK {
			continue
		}

		details := adapterDetails{
			luid:         aa.Luid,
			ifType:       aa.IfType,
			friendlyName: windows.UTF16PtrToString(aa.FriendlyName),
			description:  windows.UTF16PtrToString(aa.Description),
			gateway:      firstUsableIPv4Gateway(aa.FirstGatewayAddress),
		}

		for ua := aa.FirstUnicastAddress; ua != nil; ua = ua.Next {
			ip := usableIPv4FromSocketAddress(&ua.Address)
			if ip == nil {
				continue
			}
			lookup[ip.String()] = details
		}
	}

	return lookup
}

func firstUsableIPv4Gateway(addr *windows.IpAdapterGatewayAddress) string {
	for ga := addr; ga != nil; ga = ga.Next {
		ip := usableIPv4FromSocketAddress(&ga.Address)
		if ip != nil {
			return ip.String()
		}
	}
	return ""
}

func usableIPv4FromSocketAddress(addr *windows.SocketAddress) net.IP {
	if addr == nil {
		return nil
	}
	ip := addr.IP()
	if ip == nil {
		return nil
	}
	ipv4 := net.IP(ip).To4()
	if ipv4 == nil || ipv4.IsLoopback() || ipv4.IsLinkLocalUnicast() {
		return nil
	}
	return ipv4
}

func buildNetworkName(details adapterDetails, wifiSSID string) string {
	description := firstNonEmpty(details.description, details.friendlyName)
	if details.ifType == windows.IF_TYPE_IEEE80211 {
		return firstNonEmpty(wifiSSID, description)
	}
	if description == "" {
		return ""
	}
	if details.gateway != "" {
		return fmt.Sprintf("%s • gw %s", description, details.gateway)
	}
	return description
}

func getWiFiSSIDsByLUID() (map[uint64]string, error) {
	var negotiatedVersion uint32
	var client windows.Handle
	ret, _, _ := procWlanOpenHandle.Call(
		uintptr(wlanAPIVersion2),
		0,
		uintptr(unsafe.Pointer(&negotiatedVersion)),
		uintptr(unsafe.Pointer(&client)),
	)
	if ret != 0 {
		return nil, windows.Errno(ret)
	}
	defer procWlanCloseHandle.Call(uintptr(client), 0)

	var list *wlanInterfaceInfoList
	ret, _, _ = procWlanEnumInterfaces.Call(
		uintptr(client),
		0,
		uintptr(unsafe.Pointer(&list)),
	)
	if ret != 0 {
		return nil, windows.Errno(ret)
	}
	defer procWlanFreeMemory.Call(uintptr(unsafe.Pointer(list)))

	ssids := make(map[uint64]string)
	for _, iface := range wlanInterfaceInfos(list) {
		if iface.State != wlanInterfaceStateConnected {
			continue
		}

		var dataSize uint32
		var data unsafe.Pointer
		ret, _, _ = procWlanQueryInterface.Call(
			uintptr(client),
			uintptr(unsafe.Pointer(&iface.InterfaceGUID)),
			uintptr(wlanIntfOpcodeCurrentConnection),
			0,
			uintptr(unsafe.Pointer(&dataSize)),
			uintptr(unsafe.Pointer(&data)),
			0,
		)
		if ret != 0 || data == nil {
			continue
		}

		attrs := (*wlanConnectionAttributes)(data)
		ssid := dot11SSIDString(attrs.Assoc.SSID)
		procWlanFreeMemory.Call(uintptr(data))
		if ssid == "" {
			continue
		}

		luid, err := convertInterfaceGUIDToLUID(iface.InterfaceGUID)
		if err != nil {
			continue
		}
		ssids[luid] = ssid
	}

	return ssids, nil
}

func wlanInterfaceInfos(list *wlanInterfaceInfoList) []wlanInterfaceInfo {
	if list == nil || list.NumberOfItems == 0 {
		return nil
	}
	return unsafe.Slice(&list.InterfaceInfo[0], int(list.NumberOfItems))
}

func dot11SSIDString(ssid dot11SSID) string {
	length := int(ssid.Length)
	if length <= 0 {
		return ""
	}
	if length > len(ssid.SSID) {
		length = len(ssid.SSID)
	}
	return string(ssid.SSID[:length])
}

func convertInterfaceGUIDToLUID(guid windows.GUID) (uint64, error) {
	var luid uint64
	ret, _, _ := procConvertInterfaceGuidToLuid.Call(
		uintptr(unsafe.Pointer(&guid)),
		uintptr(unsafe.Pointer(&luid)),
	)
	if ret != 0 {
		return 0, windows.Errno(ret)
	}
	return luid, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
