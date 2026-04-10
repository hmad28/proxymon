# Rencana Redesign Dashboard Proxymon (Helicopter View)

## 1. Mencegah Konten Terpotong (Helicopter View & Fixed Size)
Tujuan utama ini adalah memastikan semua elemen (Overview, Grafik, dan Interfaces) terlihat sempurna dalam ukuran window yang *fixed*, tanpa ada bagian yang terpotong keluar dari kotak seperti yang terlihat pada screenshot.

**Langkah Penyesuaian Ruang & Layout:**
- **Atas (System Overview & Proxy Endpoint):**
  - **Masalah saat ini:** Menggunakan *fixed height* `h-[180px]`, sehingga konten di bagian bawah kartu (Socks5, Status, Active Interfaces) terpotong dan tidak terlihat.
  - **Solusi:** Menghapus paksaan tinggi `h-[180px]`. Kita akan mengubahnya menjadi `h-auto` (menyesuaikan isi) atau mengatur ulang padding (`p-5` menjadi `p-4`) dan jarak (`mb-4` menjadi `mb-2`) agar seluruh metrik muat secara nyaman dalam kartu tanpa memakan terlalu banyak ruang vertikal.
- **Tengah (Traffic Monitor / Grafik):**
  - **Masalah saat ini:** Memakan terlalu banyak ruang (`flex-1` yang tidak diimbangi dengan batas atas/bawah) atau scaling SVG yang salah.
  - **Solusi:** Memberikan *min-height* atau batas tinggi konstan pada kontainer grafik (misalnya `h-[160px]`), dan memastikan margin di sekitarnya ringkas. Ini memberi kepastian ukuran sehingga tidak menekan elemen di bawahnya.
- **Bawah (Network Interfaces):**
  - **Masalah saat ini:** Tinggi dipatok tetap `h-[220px]`.
  - **Solusi:** Menjadikan kartu-kartu interface lebih pipih (mengurangi internal padding menjadi `p-2`) sehingga pada mode *fixed window*, 2 baris interface tetap muat secara utuh. Jika jumlah interface berlebih, list ini akan memiliki internal-scroll yang rapi (`overflow-y-auto`), sementara sisa window-nya tetap *fixed size*.

## 2. Menghapus Semua Tombol yang Tidak Berguna (Useless Elements)
Untuk membuat desain "Clean" dan memberikan ruang yang lebih luas untuk data trafik, semua mockup/elemen navigasi yang tidak berfungsi akan dibersihkan:
- **Hapus Sidebar Kiri (`<aside>`):** Seluruh panel navigasi vertikal di pinggir layar akan dibuang. Ini akan membebaskan *margin-left* sebesar 80px (`ml-20`), sehingga konten utama akan lebih lebar.
- **Hapus Ikon Header:** Ikon "Settings" dan "Notifications" (Bell) di bagian pojok kanan atas akan dibuang.
- **Hapus Menu Header:** Link dummy seperti "Nodes", "Traffic", "Logs", "Rules" di posisi tengah navigasi atas akan dibuang karena semuanya memutar ke dashboard utama.
- **Evaluasi Tombol Footer:** "Reset Stats" masih dipertahankan apabila dibutuhkan secara fungsional, namun ruang footer akan dirampingkan lebih lanjut (padding lebih kecil).

## 3. Tahapan Eksekusi pada Codebase

**Fase 1: Memperbaiki Template `new-design.html`**
Kita akan mengubah file dummy/template ini terlebih dahulu.
1. Hapus `<aside>` dan elemen navbar tidak penting.
2. Hapus class `ml-20` dari `<main>`, ubah layout responsif supaya mengisi 100% lebar (dikurangi padding edge).
3. Hapus kelas tinggi yang memotong konten (`h-[180px]` dan `h-[220px]`). Update ke `min-h-min flex flex-col` supaya data yang ditambahkan di dalam card muat.
4. Sesuaikan padding (`p-x`) agar ruang kosong terpakai lebih efisien.

**Fase 2: Porting Layout ke Dashboard Aplikasi**
1. Ganti semua HTML di `internal/dashboard/assets/index.html` dengan HTML dari `new-design.html` yang sudah bersih dan rapi.
2. Pindahkan styling Tailwind ke dalam `internal/dashboard/assets/style.css` atau biarkan tetap dengan properti CDN + Tailwind Config.
3. Update `app.js` untuk membidik query selector dari HTML baru. (Tanpa logika filter manual lagi, langsung iterasi `snapshot` ke UI Helicopter View yang baru).

--------------------
_Plan ini sudah menyesuaikan kebutuhan Anda untuk fokus pada Helicopter View yang pas-layar (tidak *clipping*) dan membuang elemen "sampah" (useless)._
