#!/usr/bin/env python3
# coding: utf-8
"""
Simple Web Vulnerability Scanner
Mengecek Header Server, Cookies, dan File umum.
"""

import requests
from bs4 import BeautifulSoup
from urllib.parse import urljoin, urlparse
import re

class WebScanner:
    def __init__(self, target_url):
        self.target = target_url.strip()
        # Pastikan ada http/https di depan
        if not self.target.startswith(('http://', 'https://')):
            self.target = "http://" + self.target
        
        self.session = requests.Session()
        self.headers = {
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36'
        }

    def check_headers(self):
        """Menganalisis Header Server dan Teknologi Web"""
        print(f"\n[*] Menganalisis Header untuk: {self.target}")
        try:
            resp = self.session.get(self.target, headers=self.headers, timeout=5)
            
            print("\n[+] Hasil Analisis Header:")
            if 'Server' in resp.headers:
                server = resp.headers['Server']
                # Cek jenis web server umum
                if 'Apache' in server: print(f"    [INFO] Web Server: Apache ({server})")
                elif 'Nginx' in server: print(f"    [INFO] Web Server: Nginx ({server})")
                elif 'Microsoft-IIS' in server: print(f"    [INFO] Web Server: Microsoft IIS ({server})")
                else: print(f"    [INFO] Web Server: {server}")

            if 'X-Powered-By' in resp.headers:
                tech = resp.headers['X-Powered-By']
                print(f"    [INFO] Technology Stack: {tech} (Bisa jadi PHP/ASP.NET)")

            if 'Set-Cookie' in resp.headers:
                # Cek cookie tanpa HttpOnly atau Secure flag (kelemahan umum)
                cookies = resp.headers.get_list('Set-Cookie', [])
                print(f"    [INFO] Cookies detected: {len(cookies)}")
                for c in cookies:
                    if 'httponly' not in c.lower():
                        print(f"       - Potensi lemah (kurang HttpOnly): {c[:40]}...")

            if resp.status_code == 500:
                print("\n    [CRITICAL] Server Error 500 detected! Mungkin ada bug di endpoint target.")
                
            # Cek versi PHP/WordPress jika header terbuka (sering terjadi)
            if 'php' in str(resp.headers).lower():
                print("    [NOTE] Kemungkinan menggunakan backend PHP.")

        except requests.exceptions.Timeout:
            print("\n[!] Timeout: Server terlalu lambat membalas.")
        except requests.exceptions.ConnectionError:
            print("\n[!] Connection Error: Cek koneksi atau URL.")
        except Exception as e:
            print(f"\n[!] Error lain: {e}")

    def check_files(self):
        """Mencari file konfigurasi umum yang sering terbuka"""
        files_to_check = ['/robots.txt', '/admin', '/login.php', '/.git/config', '/wp-admin', '/backup.sql']
        found_files = []

        print(f"\n[*] Mengecek file umum di: {self.target}")
        
        for filepath in files_to_check:
            full_url = urljoin(self.target, filepath)
            try:
                resp = self.session.head(full_url, headers=self.headers, timeout=3, allow_redirects=True)
                if resp.status_code == 200:
                    print(f"    [OK] File ditemukan di: {filepath} (Status: {resp.status_code})")
                    found_files.append(filepath)
                elif resp.status_code == 403:
                    print(f"    [!] Akses Ditolak (403): {filepath}")
            except Exception:
                pass
        
        if not found_files:
            print("    [-] Tidak ada file umum ditemukan (atau di-block).")

    def simple_screenshot_html(self):
        """Mengambil halaman utama untuk dianalisis (sementara)"""
        try:
            resp = self.session.get(self.target, headers=self.headers, timeout=5)
            soup = BeautifulSoup(resp.text, 'html.parser')
            
            # Cek title
            if soup.title:
                print(f"\n[INFO] Halaman Title: {soup.title.string[:50]}")
            
            # Cek meta tags (sering digunakan untuk SEO atau debug info)
            meta_desc = soup.find('meta', attrs={'name': 'description'})
            if meta_desc and meta_desc.get('content'):
                print(f"[INFO] Meta Description: {meta_desc['content'][:50]}")
                
        except Exception as e:
            pass

    def run(self):
        self.check_headers()
        self.check_files()
        self.simple_screenshot_html()

if __name__ == "__main__":
    target = input("\n[*] Masukkan URL Target (contoh: example.com atau http://google.com): ").strip()
    
    if not target:
        print("Target kosong!")
        sys.exit()

    scanner = WebScanner(target)
    print(f">>> Memulai serangan ke {target}...")
    scanner.run()
    print("\n[+] Scan selesai.")