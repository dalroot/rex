# ⚡ راهنمای جامع فارسی پروتکل REX (نسخه 2.5 - RXP/2.5 WarpGate)

<p align="center">
  <img src="https://img.shields.io/github/v/release/dalroot/rex?style=for-the-badge&color=7289da&label=RXP%2F2.5" alt="Release">
  <img src="https://img.shields.io/badge/Latency-زیر%20۱%20میلی%20ثانیه-brightgreen?style=for-the-badge&logo=speedtest" alt="Latency">
  <img src="https://img.shields.io/badge/Transport-Enforced%20TLS1.3-blue?style=for-the-badge&logo=linux" alt="Transport">
  <img src="https://img.shields.io/badge/CLI-100%25%20Native%20Go-cyan?style=for-the-badge&logo=go" alt="Native Go CLI">
  <img src="https://img.shields.io/badge/Local%20Footprint-Zero-orange?style=for-the-badge" alt="Zero Local Footprint">
  <img src="https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge" alt="License">
</p>

<p align="center">
  <b>پروتکل اختصاصی، نیتیو و رانتایم مستقیم کنترل زیرساخت سرورها برای انسان و ایجنت‌های هوش مصنوعی.</b><br>
  <i>زمان پاسخگویی زیر ۱ میلی‌ثانیه، کلاینت ۱۰۰٪ نیتیو Go، ترمینال تعاملی مشابه SSH، سسشن‌های PTY پایدار و رمزنگاری اجباری TLS 1.3.</i>
</p>

---

<div dir="rtl" style="font-family: 'Vazirmatn', sans-serif; line-height: 2.2; text-align: right;">

## ⚡ قابلیت‌های جدید در نسخه v2.5.0 (بروزرسانی REX Connect / WarpGate)

- **کلاینت ۱۰۰٪ نیتیو به زبان گو (`rex-cli`):** بدون کوچک‌ترین وابستگی به پایتون یا پکیج‌های اضافی؛ یک فایل باینری مستقل و سبک با سرعت استارت زیر یک میلی‌ثانیه.
- **ترمینال تعاملی مشابه SSH برای کاربر انسانی (`rex connect`):** با دستور `rex @root <IP>` مستقیماً وارد شل لینوکس سرور شوید با پشتیبانی کامل از کلید **Tab** (تکمیل خودکار دستورات)، کلیدهای جهتی، و ادیتورهای تمام‌صفحه مثل **`nano`**، **`vim`** و **`htop`**.
- **ورود امن توکن بدون نمایش در ترمینال:** مشابه درخواست پسورد SSH، توکن با امنیت خوانده شده و در تاریخچه شل ذخیره نمی‌شود.
- **ارتباط امن اجباری با TLS 1.3:** تمام فریم‌های باینری روی پورت ۷۴۴۴ با مدرن‌ترین استاندارد رمزنگاری محافظت می‌شوند.
- **موتور نیتیو انتقال فایل:** آپلود و دانلود فایل‌ها بدون دردسرهای کدگذاری Base64 یا بافرینگ‌های طولانی.

---

### 💻 اتصال به سرور با دستورات ساده (مشابه SSH):

```bash
# اتصال با ساختار آشنای SSH (درخواست امن توکن در مرحله بعد)
rex @root 5.202.5.134
rex root@5.202.5.134:7444

# اتصال با ارسال مستقیم توکن یا از طریق متغیر محیطی REX_TOKEN
rex connect 5.202.5.134:7444 --token <TOKEN>

# اجرای آنی یک دستور در سرور برای اسکریپت‌های ایجنت
rex exec 5.202.5.134:7444 -t <TOKEN> "systemctl status x-ui"
```

---

## 💡 پروتکل REX چیست؟

**REX** (Remote EXecution Protocol) یک پروتکل ارتباطی اپن‌سورس و نیتیو برای **AI Agentها** (مانند هرمس، آنتی‌گریویتی، AutoGPT و سایر ایجنت‌های هوشمند) است که به آن‌ها اجازه می‌دهد **بدون درگیری سیستم محلی، ترمینال محلی یا ساخت فایل اسکریپت**، مستقیماً روی سرورهای لینوکس دستورات را در سسشن شل تعاملی یا از طریق سیستم‌کال‌های نیتیو اجرا کنند.

برعکس پروتکل‌های قدیمی که برای اپراتور انسانی طراحی شده‌اند (مثل SSH)، یا APIهای وب عمومی (REST، gRPC)، نسخه جدید REX 2.0 یک **رانتایم مستقیم در حافظه سرور (In-Memory Direct Server Runtime)** با فریم‌بندی باینری ۸ بایتی و قابلیت خودراهنمایی هوش مصنوعی ارائه می‌دهد.

```
 [ ایجنت هوش مصنوعی / سیستم کاربر ] 
                  │
                  │  (سوکت باینری TCP پورت 7444 / هدر فشرده ۸ بایتی)
                  ▼
┌─────────────────────────────────────────────────────────────┐
│                 رانتایم مستقیم سرور (rex-node)               │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ 1. RXP Binary Protocol Decoder (پردازش فریم‌ها)        │  │
│  └───────────────────────────────────────────────────────┘  │
│                              │                              │
│       ┌──────────────────────┼──────────────────────┐       │
│       ▼                      ▼                      ▼       │
│ ┌───────────────┐   ┌─────────────────┐   ┌──────────────┐  │
│ │ 2. PTY Engine │   │ 3. Native Engine│   │ 4. AI Guide  │  │
│ │ - شل زنده سرور│   │ - فایل‌ها        │   │ - قانون کلی  │  │
│ │ - حفظ cd و env│   │ - سخت‌افزار      │   │ - پدیده Zero │  │
│ └───────────────┘   └─────────────────┘   └──────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

---

## ⚔️ جدول مقایسه معماری REX 2.0 با SSH و وب‌سوکت

<table dir="rtl" style="width:100%; border-collapse:collapse; text-align:right;">
<thead>
<tr style="background:#2d2d2d; color:#fff;">
<th style="padding:10px; border:1px solid #444;">معیار / ویژگی</th>
<th style="padding:10px; border:1px solid #444;">SSH / TTY سنتی</th>
<th style="padding:10px; border:1px solid #444;">WebSockets / REST</th>
<th style="padding:10px; border:1px solid #444; background:#1b4332;">💡 REX 2.0 (Raw TCP Binary)</th>
</tr>
</thead>
<tbody>
<tr>
<td style="padding:10px; border:1px solid #444;"><b>ردپای سیستم محلی</b></td>
<td style="padding:10px; border:1px solid #444;">نیازمند TTY محلی و شل واسط</td>
<td style="padding:10px; border:1px solid #444;">ساخت Subprocess و فایل موقت</td>
<td style="padding:10px; border:1px solid #444; color:#2ec4b6;"><b>صفر (فقط سوکت باینری شبکه)</b></td>
</tr>
<tr>
<td style="padding:10px; border:1px solid #444;"><b>اورهد هدر شبکه</b></td>
<td style="padding:10px; border:1px solid #444;">سنگین (دست دادن‌های SSH)</td>
<td style="padding:10px; border:1px solid #444;">سنگین (هدرهای HTTP و متن JSON)</td>
<td style="padding:10px; border:1px solid #444; color:#2ec4b6;"><b>فوق‌العاده کم (هدر باینری ۸ بایتی)</b></td>
</tr>
<tr>
<td style="padding:10px; border:1px solid #444;"><b>تداوم وضعیت (State Continuity)</b></td>
<td style="padding:10px; border:1px solid #444;">قطع با بسته شدن اتصال</td>
<td style="padding:10px; border:1px solid #444;">Stateless (ریست شدن <code>cd</code> و متغیرها)</td>
<td style="padding:10px; border:1px solid #444; color:#2ec4b6;"><b>حفظ ۱۰۰٪ وضعیت در PTY سرور</b></td>
</tr>
<tr>
<td style="padding:10px; border:1px solid #444;"><b>عملیات فایل و سخت‌افزار</b></td>
<td style="padding:10px; border:1px solid #444;">نیازمند کامندهای <code>cat</code> و <code>ls</code></td>
<td style="padding:10px; border:1px solid #444;">کدگذاری Base64 و انکودینگ متن</td>
<td style="padding:10px; border:1px solid #444; color:#2ec4b6;"><b>سیستم‌کال‌های مستقیم OS در حافظه</b></td>
</tr>
<tr>
<td style="padding:10px; border:1px solid #444;"><b>تاخیر شروع اجرا (Latency)</b></td>
<td style="padding:10px; border:1px solid #444;">۵۰ms تا ۲۵۰ms</td>
<td style="padding:10px; border:1px solid #444;">۱۰ms تا ۴۰ms</td>
<td style="padding:10px; border:1px solid #444; color:#2ec4b6;"><b>زیر ۱.۸ میلی‌ثانیه (< 1.8ms)</b></td>
</tr>
</tbody>
</table>

---

## 🎯 ۴ رکن اصلی معماری RXP/2.0

### ۱. عدم درگیری سیستم محلی (Zero-Local Footprint)
ایجنت روی ماشین کاربر **هیچ ترمینال، شل یا فایل اسکریپت موقتی اجرا نمی‌کند**. ارتباط ۱۰۰٪ مستقیم از طریق سوکت باینری شبکه روی پورت `7444` انجام می‌شود.

### ۲. سسشن شل تعاملی پایدار (Persistent Server PTY Shell)
یک سسشن `/bin/bash` زنده روی سرور باز می‌ماند؛ در نتیجه تغییر مسیرها (`cd /var/www`)، متغیرهای محیطی (`export DB_PASS=123`) و پروسه‌ها به صورت دائم حفظ می‌شوند.

### ۳. موتور نیتیو سیستم‌کال (Native Syscall Engine)
برای کارهای عمومی مثل خواندن/نوشتن فایل یا مشاهده سخت‌افزار، سیستم‌کال مستقیم Go در حافظه اجرا شده و هیچ فرایند شلی صدا زده نمی‌شود.

### ۴. اپ‌کد خودراهنمایی ایجنت (`OpAgentGuide` / `0x0A`)
پروتکل به محض اتصال یا استعلام ایجنت، قانون کلی را بازمی‌گرداند:
```text
[REX PROTOCOL RULE]
You are connected to the REX Direct Server Runtime via RXP/2.0.
CRITICAL DIRECTIVE:
1. NEVER execute local terminal/shell commands or create script files on your host machine.
2. ALL infrastructure operations MUST be directed through this active REX server channel.
```

---

## 📐 ساختار فریم باینری اختصاصی (RXP/2.0 Wire Specification)

تمام ارتباطات شبکه روی **پورت باینری 7444** از هدر فشرده ۸ بایتی استفاده می‌کنند:

```text
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|       Magic Bytes ('R' 'X')   |    Ver (1B)   |  Opcode (1B)  |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|          Stream ID (2B)       |      Payload Length (2B)      |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                                                               |
|                       Payload (Raw Bytes)                     |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

### 🏷️ جدول کدهای عملیاتی (Opcodes):

<table dir="rtl" style="width:100%; border-collapse:collapse; text-align:right;">
<thead>
<tr style="background:#2d2d2d; color:#fff;">
<th style="padding:8px; border:1px solid #444;">Opcode</th>
<th style="padding:8px; border:1px solid #444;">عنوان کد</th>
<th style="padding:8px; border:1px solid #444;">جهت</th>
<th style="padding:8px; border:1px solid #444;">توضیحات عملکرد</th>
</tr>
</thead>
<tbody>
<tr><td style="padding:8px; border:1px solid #444;"><code>0x01</code></td><td style="padding:8px; border:1px solid #444;"><code>OP_AUTH_HANDSHAKE</code></td><td style="padding:8px; border:1px solid #444;">دو طرفه</td><td style="padding:8px; border:1px solid #444;">احراز هویت توکن امنیتی باینری</td></tr>
<tr><td style="padding:8px; border:1px solid #444;"><code>0x02</code></td><td style="padding:8px; border:1px solid #444;"><code>OP_PTY_SPAWN</code></td><td style="padding:8px; border:1px solid #444;">کلاینت ➔ سرور</td><td style="padding:8px; border:1px solid #444;">ساخت سسشن شل پایدار تعاملی در سرور</td></tr>
<tr><td style="padding:8px; border:1px solid #444;"><code>0x03</code></td><td style="padding:8px; border:1px solid #444;"><code>OP_PTY_DATA</code></td><td style="padding:8px; border:1px solid #444;">دو طرفه</td><td style="padding:8px; border:1px solid #444;">استریم زنده بایت‌های STDIN و STDOUT</td></tr>
<tr><td style="padding:8px; border:1px solid #444;"><code>0x04</code></td><td style="padding:8px; border:1px solid #444;"><code>OP_PTY_RESIZE</code></td><td style="padding:8px; border:1px solid #444;">کلاینت ➔ سرور</td><td style="padding:8px; border:1px solid #444;">تغییر ابعاد ترمینال سرور (سطر و ستون)</td></tr>
<tr><td style="padding:8px; border:1px solid #444;"><code>0x05</code></td><td style="padding:8px; border:1px solid #444;"><code>OP_PTY_CLOSE</code></td><td style="padding:8px; border:1px solid #444;">دو طرفه</td><td style="padding:8px; border:1px solid #444;">بستن سسشن شل سرور</td></tr>
<tr><td style="padding:8px; border:1px solid #444;"><code>0x06</code></td><td style="padding:8px; border:1px solid #444;"><code>OP_NATIVE_FILE_OP</code></td><td style="padding:8px; border:1px solid #444;">دو طرفه</td><td style="padding:8px; border:1px solid #444;">سیستم‌کال نیتیو فایل (خواندن، نوشتن، وضعیت)</td></tr>
<tr><td style="padding:8px; border:1px solid #444;"><code>0x07</code></td><td style="padding:8px; border:1px solid #444;"><code>OP_NATIVE_SYSINFO</code></td><td style="padding:8px; border:1px solid #444;">دو طرفه</td><td style="padding:8px; border:1px solid #444;">استعلام نیتیو رم، سی‌پیو و آپ‌تایم سرور</td></tr>
<tr><td style="padding:8px; border:1px solid #444;"><code>0x0A</code></td><td style="padding:8px; border:1px solid #444;"><code>OP_AGENT_GUIDE</code></td><td style="padding:8px; border:1px solid #444;">دو طرفه</td><td style="padding:8px; border:1px solid #444;">دریافت قانون کلی پروتکل توسط ایجنت</td></tr>
</tbody>
</table>

---

## ⚡ نصب و راه‌اندازی سرور (`rex-node`)

### ۱. نصب تک‌دستور روی سرور لینوکس

```bash
curl -fsSL https://raw.githubusercontent.com/dalroot/rex/master/rex-node/install.sh | bash
```

### ۲. مدیریت سطح دسترسی با CLI (`rex mode`)

```bash
# مشاهده مود فعال فعلی
rex mode

# تغییر به مود خودمختار کامل (آزادی عمل ایجنت با مسدودی دستورات تخریبی)
rex mode autonomous

# تغییر به مود بازبینی (دستورات خواندنی آزاد، تغییردهنده مسدود)
rex mode review

# تغییر به مود لیست سفید سخت‌گیرانه
rex mode allowlist
```

---

## 🚀 راهنمای کد پایتون (`RXPDirectClient`)

### نصب پکیج

```bash
pip install git+https://github.com/dalroot/rex.git#subdirectory=rex-client
```

### نمونه کد کامل پایتون

```python
import asyncio
from rex_client import RXPDirectClient

async def main():
    # 1. اتصال مستقیم سوکت باینری TCP روی پورت 7444 (بدون درگیری شل محلی)
    async with RXPDirectClient(host="80.253.254.207", token="YOUR_TOKEN", port=7444) as client:
        
        # 2. دریافت قانون کلی پروتکل REX
        directive = await client.get_agent_guide()
        print(f"=== قانون کلی REX ===\n{directive}\n")

        # 3. ایجاد سسشن شل پایدار در سرور (حفظ کامل cd و متغیرها)
        session = await client.spawn_pty(cols=80, rows=24)
        
        await session.send("cd /var/www/html\n")
        await session.send("export DEPLOY_ENV=production\n")
        await session.send("pwd && echo $DEPLOY_ENV\n")
        
        stdout = await session.read_output(timeout=0.5)
        print(f"=== خروجی شل سرور ===\n{stdout}")
        await session.close()

        # 4. سیستم‌کال نیتیو فایل روی سرور (بدون لود کردن شل)
        file_stat = await client.native_file_op("stat", "/etc/passwd")
        print(f"وضعیت فایل: {file_stat['stat']}")

        # 5. استعلام رم و سخت‌افزار سرور
        info = await client.sysinfo()
        print(f"رم در دسترس: {info['mem_available_kb']} KB | تعداد هسته: {info['cpus']}")

if __name__ == "__main__":
    asyncio.run(main())
```

---

## 📄 لایسنس
MIT License © 2026 dalroot & REX Community.

</div>
