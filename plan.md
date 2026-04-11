# Rencana Perbaikan Auto-Detect Network Interface (Hotplug/WiFi Change)

## Ringkasan Masalah
Saat ini, aplikasi `multi-isp-proxy` (Proxymon) hanya melakukan deteksi antarmuka jaringan (network interface) beserta alamat IP-nya **satu kali** pada saat aplikasi pertama kali dijalankan (di dalam `app.NewController`). 

Akibatnya:
1. Jika pengguna pindah koneksi Wi-Fi (IP berubah), aplikasi masih menyimpan status IP lama yang menyebabkan antarmuka mati/tidak stabil di level Proxy.
2. `PerInterfaceTracker` memonitor *LUID* antarmuka secara statis berdasarkan data saat *startup*. Jika ada *interface* baru yang diaktifkan (misal tethering lewat USB), ia tidak akan pernah terlacak trafiknya.
3. Kunci identifikasi interface (`InterfaceKey`) menggunakan format `[Nama Interface]|[IP Address]`. Jika IP Address berubah saat *reconnect* Wi-Fi, aplikasi menganggapnya sebagai interface target yang sama sekali berbeda, sehingga interface gagal terpilih (hilang).

## Solusi Teknis (Implementation Plan)

### Fase 1: Membuat Identifikasi Interface yang Stabil
- **Target File:** `internal/app/controller.go`
- **Tindakan:** Mengubah `InterfaceKey(iface *netif.NetInterface) string`. Hilangkan `iface.IP.String()` dan gunakan `iface.Name` (contoh: "Wi-Fi" atau "Ethernet") sebagai kunci unik statis. 
*(Windows menggunakan identifikasi nama interface yang persisten. Jika IP berubah tetapi fisik interface-nya sama, ia tetap mempertahankan nama "Wi-Fi").*

### Fase 2: Dynamic Interface Discovery di Controller
- **Target File:** `internal/app/controller.go`
- **Tindakan:** 
  1. Hentikan pemanggilan `go netif.Monitor(...)` yang ada di `startLocked()`. Fungsi lama ini hanya mem-ping list device yang sifatnya statis.
  2. Implementasikan routine baru `go c.watchInterfaces(runCtx)` di dalam `Controller`. Routine ini akan berjalan berkala (setiap 5-10 detik) untuk:
     - Memanggil `netif.Discover()` agar mendapat daftar antarmuka / IP Address yang terbaru.
     - Memanggil `CheckHealth` untuk memastikan status online (`Alive`).
     - Memeluk *Lock* (`c.mu.Lock()`) dan melakukan penggantian `c.allIfaces` secara transparan.

### Fase 3: Update Tracker & Balancer Secara Dinamis (No App Restart)
- **Target File:** `internal/app/controller.go` dan `internal/netif/traffic_windows.go`
- **Tindakan:** 
  - Saat `c.allIfaces` terganti dalam loop `watchInterfaces`, kita mengkueri interface yang aktif `c.selectedIfacesLocked()` dengan IP baru.
  - Memanggil `c.bal.SetInterfaces(newSelected)` untuk *hot-reload* konfigurasi balancer supaya load balancer me-link ke IP baru secara instan.
  - Memodifikasi `PerInterfaceTracker` agar loop `Monitor`-nya dapat menerima update list kumpulan LUID (ID network adapter) yang dinamis. 

---
*Bila setuju dengan pendekatan ini, kamu tidak perlu memencet Reset App lagi waktu berpindah Wi-Fi. Semua layer data, grafik, balancer akan otomatis sinkron mengikut state jaringan lokal kamu!*
