<div align="center">

<a href="README.md">EN</a> | <a href="README.tr.md">TR</a>

# dna — DomainNameAPI CLI

**Alan adı, DNS, DNSSEC, SSL ve hosting işlemleri: terminalden, betiklerden ve yapay zeka ajanlarından.**

[![CI](https://github.com/domainreseller/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/domainreseller/cli/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

</div>

`dna`, [DomainNameAPI](https://www.domainnameapi.com) bayi (reseller) REST API'sinin resmi komut satırı istemcisidir.
Canlı ortam `api.domainresellerapi.com`, test ortamı (OTE) `ote.domainresellerapi.com` adresindedir.

- API'deki **64 operasyonun tamamını** kapsar.
- Yerleşik bir **MCP sunucusu** vardır, böylece Claude, Cursor, Codex gibi asistanlar da `dna`'yı kullanabilir.

```console
$ dna domain search example.com
$ dna domain list --all --jq '.items[] | select(.remainingDay < 30) | .domainName'
$ dna dns add example.com --name www --type A --content 192.0.2.10
$ dna domain renew example.com --period 1 --dry-run
```

## Neden dna?

**Yapay zeka (AI) için**
- Çıktı borulandığında (pipe) JSON üretir.
- Hatalar stderr'e JSON olarak ve [sabit çıkış kodlarıyla](docs/errors.md) yazılır.
- `dna schema <op>` her girdiyi örnekle birlikte JSON Schema olarak verir.
- `dna mcp serve` her operasyonu güvenlik işaretleriyle birlikte bir MCP aracı olarak sunar.
- Ücretli ve yıkıcı işlemler `--yes` (MCP'de `confirm=true`) olmadan çalışmaz.
- `--dry-run` gönderilecek HTTP isteğini birebir gösterir, `--read-only` her türlü değişikliği engeller.
- `dna agent guide`, bir ajana tüm CLI'yı tek çağrıda öğretir.

**Geliştirici için**
- Terminalde tablo çıktısı verir. `--jq`, `-o jsonl`, `--all` (tüm sayfalar), `--body @dosya.json` ve stdin desteklenir.
- Canlı ve OTE için profiller ile kabuk tamamlama (shell completion) vardır.
- Hatalar ipucu ve correlation id ile okunaklı gelir.
- Kota aşımında (rate limit) otomatik yeniden dener ve gateway limitlerini aşmamak için istekleri aralıklı gönderir.

**Her zaman eksiksiz**
- Komutlar, flag'ler, MCP araçları ve [referans dokümanları](docs/reference/README.md) API'nin OpenAPI dokümanından üretilir.
- Eksik kalan bir operasyon olursa test başarısız olur.

## Kurulum

**Go 1.26+ ile:**

```bash
go install github.com/domainreseller/cli/cmd/dna@latest
```

**Hazır derlenmiş dosyalar:** Linux, macOS ve Windows (amd64/arm64) için [Releases](https://github.com/domainreseller/cli/releases/latest) sayfasında bulunur. Kurulum betikleriyle de kurabilirsiniz:

```bash
curl -fsSL https://raw.githubusercontent.com/domainreseller/cli/main/scripts/install.sh | sh
```

```powershell
irm https://raw.githubusercontent.com/domainreseller/cli/main/scripts/install.ps1 | iex
```

**Kaynaktan:** `git clone https://github.com/domainreseller/cli && cd cli && make install`

Kurulumu `dna version` ve `dna doctor` ile kontrol edin.

## Hızlı başlangıç

Bir **Reseller ID** (UUID) ve bir **API anahtarı** gerekir. İkisi de bayi panelinde **API Ayarları** altındadır.
Canlı ortam ve OTE (test) ayrı ortamlardır ve **API anahtarları farklıdır**.

```bash
# 1. Önce test ortamına giriş yapın. Anahtar gizli girilir ve kimlik bilgileri doğrulanır.
dna auth login --env ote

# 2. Keşfedin (hepsi salt-okunur)
dna --ote account get                       # bakiye
dna --ote domain search example.com         # müsaitlik + fiyat
dna --ote product tlds --filter-text com.tr # uzantı kuralları ve fiyatları

# 3. Değişiklik yapın. Ücretli komutlar onay ister; --dry-run isteği önizler.
dna --ote domain register example.com --ns ns1.example.net,ns2.example.net --handle DNA-12345 --dry-run
dna --ote domain register example.com --ns ns1.example.net,ns2.example.net --handle DNA-12345

# 4. Canlı ortam
dna auth login              # "production" profiline kaydedilir
dna domain list
```

CI ve container ortamlarında yapılandırma dosyası yerine ortam değişkenlerini kullanın:

```bash
export DNA_RESELLER_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
export DNA_API_KEY=...        # secret store'unuzda saklayın
export DNA_ENV=ote            # veya production (varsayılan)
dna domain list -o json
```

Profiller, öncelik sırası ve tüm ortam değişkenleri için [yapılandırma](docs/configuration.md) sayfasına bakın.

## Komutlar

| grup | ne yapar |
|---|---|
| `dna account` | bakiye, hesap hareketleri |
| `dna domain` | arama, toplu arama, listeleme, detay, kayıt, yenileme, transfer, geri yükleme, kilit, gizlilik, auth kodu, kişiler, belgeler, yönlendirme, nameserver, child host |
| `dna dns` | zone kayıtları: list, add, update, delete, apply |
| `dna dnssec` | DS kayıtları: get, add, remove, disable |
| `dna contact` | kişi (handle) kayıtları ve doğrulama |
| `dna ssl` | CSR, satın alma, yapılandırma, durum, indirme, yeniden düzenleme, iptal |
| `dna hosting` | paketler, planlar, oluşturma, durum |
| `dna product` | ürünler, fiyatlar, uzantı (TLD) kataloğu, SSL kataloğu |
| `dna order` | sipariş detayı |
| `dna api` | herhangi bir uç noktaya ham istek (`gh api` gibi) |
| `dna schema`, `dna enums` | operasyonların ve kodlu değerlerin makine-okunur tarifi |
| `dna mcp`, `dna agent` | MCP sunucusu, ajan rehberi, Claude Code skill dosyası |
| `dna auth`, `dna config`, `dna doctor` | kimlik bilgileri, profiller, teşhis |

Her komutun `--help` çıktısında güvenlik sınıfı, API yolu ve örnekler bulunur. Tam liste [komut referansındadır](docs/reference/README.md).

## Güvenlik modeli

Her operasyonun bir güvenlik sınıfı vardır:

| sınıf | örnek | davranış |
|---|---|---|
| `read` | arama, listeleme, detay, fiyat | hemen çalışır |
| `write` | DNS değişikliği, kilit, gizlilik, kişiler | hemen çalışır (ücretsiz, geri alınabilir) |
| `destructive` | kayıt silme, SSL iptali, DNSSEC kapatma | onay ister; etkileşimsiz kullanımda `--yes` gerekir |
| `billable` | kayıt, yenileme, transfer, geri yükleme, SSL/hosting satın alma | onay ister; etkileşimsiz kullanımda `--yes` gerekir |

- Terminal yoksa ve `--yes` verilmemişse, ücretli veya yıkıcı bir komut **9 koduyla** çıkar ve hiçbir şey göndermez.
- `--dry-run` gönderilecek isteği API anahtarı maskelenmiş olarak yazdırır.
- `--read-only`, `DNA_READ_ONLY=1` veya salt-okunur bir profil her değişikliği engeller (çıkış kodu 10).

## Yapay zeka asistanlarıyla kullanım

```bash
# Claude Code: MCP sunucusunu ekleyin (OTE ve salt-okunur iyi bir başlangıç)
claude mcp add dna -- dna mcp serve --env ote --read-only

# Claude Desktop, Cursor, VS Code, Codex, Gemini CLI, Windsurf için hazır ayarlar
dna mcp config

# Ya da ajana doğrudan CLI'yı ve skill/rehber dosyasını verin
dna agent skill --install          # ~/.claude/skills/dna/SKILL.md
dna agent guide                    # her ajan için kısa kullanım rehberi
```

MCP sunucusu güvenliği şöyle yönetir:
- Ücretli ve yıkıcı araçlar yalnızca `confirm=true` ile çalışır. Bu olmadan asistanın size gösterebileceği bir önizleme döner.
- `--read-only`, `--no-billable` ve `--groups domain,dns` asistanın erişimini sınırlar.

Ayrıntılar: [AI ajanları](docs/ai-agents.md).

## Çıktı ve betik yazma

```bash
dna domain info example.com                  # terminalde tablo
dna domain info example.com -o json          # JSON (borulandığında varsayılan)
dna domain list --all -o jsonl               # her satırda bir alan adı
dna domain info example.com --jq .authCode   # jq filtresi; metinler tırnaksız basılır
dna dns add --body @kayit.json               # tüm girdi dosyadan (stdin için --body -)
dna enums DomainStatus                       # 7 numaralı durum ne demek
```

Çıkış kodları sabittir (bkz. [hatalar](docs/errors.md)):

| kod | anlamı |
|---|---|
| 0 | başarılı |
| 2 | geçersiz girdi; hiçbir şey gönderilmedi |
| 3 | kimlik doğrulama |
| 4 | bulunamadı |
| 5 | API reddetti |
| 6 | kota aşıldı (rate limit) |
| 7 | sunucu hatası |
| 8 | ağ |
| 9 | onay gerekli |
| 10 | salt-okunur |

## Dokümantasyon

Ayrıntılı dokümanlar İngilizcedir:

- [Başlarken](docs/getting-started.md)
- [Yapılandırma ve kimlik bilgileri](docs/configuration.md)
- [Kullanım: girdi, çıktı, betikler](docs/usage.md)
- [AI ajanları ve MCP](docs/ai-agents.md)
- [Hatalar ve çıkış kodları](docs/errors.md)
- [Komut referansı](docs/reference/README.md) ve [enum listesi](docs/enums.md) (otomatik üretilir)
- [Mimari ve katkı](docs/architecture.md), [CONTRIBUTING.md](CONTRIBUTING.md)

## Lisans

[Apache 2.0](LICENSE)
