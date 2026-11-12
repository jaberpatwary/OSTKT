# 🎫 Auto Ticket System

শুধু নাম লিখুন — বাকিটা অটো। Excel থেকে employee-এর তথ্য অটো ফিল হয়, **DONE** চাপলে টিকিট সেভ হয়ে employee-এর ইমেইলে মেইল যায়, **PENDING** চাপলে শুধু সেভ হয়।

পুরো স্ট্যাক ফ্রি: Go + Gin + GORM, Neon PostgreSQL (লোকালে SQLite), Brevo SMTP, Render হোস্টিং, Tailwind CDN।

---

## 📁 প্রজেক্ট স্ট্রাকচার

```
ticket-system/
├── main.go            ← পুরো ব্যাকএন্ড (মডেল, হ্যান্ডলার, Excel, ইমেইল)
├── go.mod             ← ডিপেন্ডেন্সি
├── .env.example       ← কপি করে .env বানান
├── .gitignore
├── employees.xlsx     ← Local employee workbook (ignored by Git; do not commit real staff data)
└── static/
    └── index.html     ← পুরো UI (Tailwind CDN + vanilla JS)
```

---

## 🚀 লোকালে চালানো (৫ মিনিটে)

**১. Go ইনস্টল করুন** — https://go.dev/dl থেকে সর্বশেষ ভার্সন (1.22 বা তার বেশি)। চেক করুন:

```bash
go version
```

**২. ডিপেন্ডেন্সি নামান** (প্রথমবার একবার):

```bash
cd ticket-system
go mod tidy
```

এটা `go.sum` তৈরি করবে এবং নিচের প্যাকেজগুলো নামাবে:

| প্যাকেজ | কাজ |
|---|---|
| `github.com/gin-gonic/gin` | ওয়েব ফ্রেমওয়ার্ক |
| `gorm.io/gorm` | ORM |
| `gorm.io/driver/postgres` | Neon PostgreSQL |
| `github.com/glebarez/sqlite` | লোকাল SQLite (pure Go — gcc/CGO লাগে না) |
| `github.com/xuri/excelize/v2` | Excel পড়া |
| `gopkg.in/mail.v2` | SMTP ইমেইল |
| `github.com/joho/godotenv` | `.env` পড়া |

**৩. `.env` বানান:**

```bash
cp .env.example .env

```

On a fresh clone, run `go run . -sample-excel` to create the synthetic workbook before starting the app. The real employee workbook is intentionally not included.

শুধু টেস্ট করতে চাইলে কিছুই বদলাতে হবে না — `DATABASE_URL` কমেন্ট করা থাকলে লোকাল SQLite (`tickets.db`) ব্যবহার হবে, আর SMTP ঠিক না থাকলে টিকিট সেভ হবে কিন্তু মেইল যাবে না (লগে লেখা থাকবে)।

**৪. চালান:**

```bash
go run .
```

ব্রাউজারে খুলুন: **http://localhost:8080** — "Rah" লিখে দেখুন।

---

## 📊 employees.xlsx ফরম্যাট

স্যাম্পল ফাইল দেওয়া আছে। নিজে নতুন বানাতে চাইলে:

```bash
go run . -sample-excel      # employees.xlsx না থাকলে স্যাম্পল বানায়
```

নিয়ম:

- **শুধু প্রথম শিট** পড়া হয়, **প্রথম সারি header** (স্কিপ হয়)।
- কলাম: `A: Name | B: EmployeeID | C: Department | D: Email | E: Phone`
- **Phone কলাম Text ফরম্যাটে** রাখুন (Excel-এ কলাম সিলেক্ট → Format Cells → Text)। ভুলে সংখ্যা হয়ে গেলেও বাংলাদেশি মোবাইল নম্বরের শুরুর `0` অ্যাপ নিজে ফিরিয়ে আনে।
- একই নামে একাধিক জন থাকতে পারে — **EmployeeID আলাদা রাখুন**। UI-তে তালিকায় "একই নাম" ব্যাজ দেখিয়ে ID/বিভাগ দেখে বাছতে দেয়।
- বাংলা নামও চলে (যেমন `সাকিব হাসান`)।
- Excel সেভ করলেই অ্যাপ পরের সার্চে নতুন ডেটা নিয়ে নেয় — রিস্টার্ট লাগে না।

---

## 📧 Brevo SMTP সেটআপ (ফ্রি)

1. https://www.brevo.com এ ফ্রি অ্যাকাউন্ট খুলুন।
2. **Senders, Domains & Dedicated IPs → Senders** থেকে যে ইমেইল থেকে মেইল পাঠাবেন সেটা যোগ করে **verify** করুন। নিজের ডোমেইন থাকলে সেটা authenticate করলে মেইল spam-এ কম যায়।
3. **SMTP & API → SMTP** ট্যাবে যান:
   - **Login** (যেমন `8a1b2c001@smtp-brevo.com`) → `SMTP_USER`
   - **Generate a new SMTP key** → সেই key → `SMTP_PASS` (এটা API key নয়!)
4. `.env`-এ বসান:

```env
SMTP_HOST=smtp-relay.brevo.com
SMTP_PORT=587
SMTP_USER=8a1b2c001@smtp-brevo.com
SMTP_PASS=xsmtpsib-xxxxxxxx
SMTP_FROM=you@yourdomain.com      # ধাপ ২-এ verify করা ঠিকানা
SMTP_FROM_NAME=Auto Ticket System
```

> ⚠️ `SMTP_FROM` অবশ্যই verified sender হতে হবে। Brevo-র Login (`...@smtp-brevo.com`) দিয়ে মেইল পাঠানো যায় না।
>
> Brevo ফ্রি প্ল্যানে দৈনিক ইমেইলের একটা সীমা আছে (লেখার সময় দিনে ~৩০০টি) — সাধারণ অফিস ব্যবহারে যথেষ্ট।

---

## Groq AI Ticket Assistant

Ticket form-এর নিচের chat assistant কর্মীর নাম, সমস্যার বিবরণ এবং optional assignee বুঝে ticket তৈরি করে। Ticket সবসময় `pending` status-এ তৈরি হয়। নাম পুরোপুরি না মিললে assistant employee options দেখায়; একই নাম থাকলে ticket তৈরির আগে সঠিক employee বেছে নিতে হয়। Groq API key browser-এ যায় না; server থেকে API call করা হয়।

Groq console থেকে API key তৈরি করে local `.env` বা deployment secret-এ রাখুন:

```env
GROQ_API_KEY=your_rotated_groq_api_key
GROQ_MODEL=openai/gpt-oss-20b
```

পরিবর্তনের পর app restart করুন। API key কখনো GitHub-এ commit করবেন না। Key না থাকলে chat UI বন্ধ থাকবে; সাধারণ ticket form কাজ করবে।

---

## 🐘 Neon PostgreSQL সেটআপ (ফ্রি)

1. https://neon.tech এ ফ্রি অ্যাকাউন্ট খুলে নতুন **Project** বানান (Region: Singapore — বাংলাদেশ থেকে সবচেয়ে কাছে)।
2. Dashboard → **Connect** → connection string কপি করুন।
3. `.env`-এ বসান:

```env
DATABASE_URL=postgresql://user:pass@ep-xxx.ap-southeast-1.aws.neon.tech/neondb?sslmode=require
```

টেবিল (`tickets`) অ্যাপ চালু হলে নিজে থেকেই তৈরি হয় (GORM AutoMigrate)। Neon-এর স্ট্রিং-এ `channel_binding=require` থাকলে অ্যাপ নিজেই সেটা বাদ দেয় — কিছু করতে হবে না।

---

## ☁️ Render-এ ডিপ্লয় (ফ্রি)

Do not commit the real `employees.xlsx` workbook to this public repository; it contains employee identifiers and contact details. Store it privately and set `EXCEL_PATH` in the deployment environment.

**২.** https://render.com → **New → Web Service** → রিপো সিলেক্ট করুন:

| সেটিং | মান |
|---|---|
| Runtime / Language | Go |
| Build Command | `go build -tags netgo -ldflags '-s -w' -o app` |
| Start Command | `./app` |
| Instance Type | Free |
| Health Check Path | `/healthz` (Advanced-এ) |

**৩. Environment Variables** যোগ করুন:

```
DATABASE_URL    = (Neon-এর connection string)
SMTP_HOST       = smtp-relay.brevo.com
SMTP_PORT       = 2525        ← গুরুত্বপূর্ণ! নিচে দেখুন
SMTP_USER       = (Brevo login)
SMTP_PASS       = (Brevo SMTP key)
SMTP_FROM       = (verified sender)
SMTP_FROM_NAME  = Auto Ticket System
APP_USER        = admin               ← সাইটে লগইন
APP_PASS        = (শক্ত পাসওয়ার্ড)
GIN_MODE        = release
```

> 🚨 **Render free tier-এ SMTP port 25, 465, 587 ব্লক করা** (সেপ্টেম্বর ২০২৫ থেকে)। তাই Render-এ `SMTP_PORT=2525` দিন — Brevo এই পোর্ট সাপোর্ট করে, কোড বদলাতে হবে না। লোকালে 587 চলবে।

> 🔒 **APP_USER / APP_PASS অবশ্যই দিন।** না দিলে লিংক জানা যে কেউ employee-দের তথ্য দেখতে ও তাদের ইমেইল পাঠাতে পারবে। দিলে ব্রাউজার একবার ইউজারনেম-পাসওয়ার্ড চাইবে।

**৪. Deploy** চাপুন। কয়েক মিনিট পর `https://your-app.onrender.com` লিংক পাবেন।

> 💤 Render free সার্ভিস ১৫ মিনিট কেউ ব্যবহার না করলে ঘুমিয়ে পড়ে — তখন প্রথম রিকোয়েস্টে ৩০-৬০ সেকেন্ড লাগতে পারে। ডেটা Neon-এ থাকে, তাই হারায় না। (লোকাল SQLite Render-এ ব্যবহার করবেন না — রিস্টার্টে মুছে যায়।)

Do not commit the real `employees.xlsx` workbook to this public repository; it contains employee identifiers and contact details. Store it privately and set `EXCEL_PATH` in the deployment environment.

---

## 🔌 API

| মেথড | পাথ | কাজ |
|---|---|---|
| GET | `/api/search?q=Rahim` | Excel থেকে খোঁজ (partial + case-insensitive, সর্বোচ্চ ৫টি) |
| POST | `/api/tickets` | টিকিট তৈরি — `{"name": "...", "status": "done" \| "pending", "employee_id": "ঐচ্ছিক", "problem": "...", "assigned_to": "ঐচ্ছিক"}` |
| PUT | `/api/tickets/:id/assignee` | টিকিট assign/update/clear — `{"assignee":"নাম"}` (খালি দিলে clear) |
| POST | `/api/tickets/:id/done` | পেন্ডিং → Done + ইমেইল (`:id` = DB id অথবা টিকিট নম্বর) |
| GET | `/api/tickets/:id` | একটি টিকিট (ইমেইল গেছে কিনা `email_sent_at` দেখে) |
| GET | `/api/tickets?status=pending&limit=50` | তালিকা (অ্যাডমিন) |
| GET | `/healthz` | হেলথ চেক |

**POST /api/tickets রেসপন্স কোড:** `201` তৈরি হয়েছে · `400` ভুল ইনপুট · `404` নাম মেলেনি · `409` একই নামে একাধিক জন (রেসপন্সে `matches` তালিকা থাকে — `employee_id` দিয়ে আবার পাঠান)

`problem` আবশ্যক (সর্বোচ্চ ২০০০ অক্ষর); `assigned_to` ঐচ্ছিক (সর্বোচ্চ ১০০ অক্ষর)।

`employee_id` দিলে সেটা দিয়েই নিখুঁতভাবে মেলানো হয়। UI সবসময় এটা পাঠায়, তাই একই নামের দুজন থাকলেও সঠিক জনের টিকিট হয়।

### curl দিয়ে টেস্ট

```bash
curl "http://localhost:8080/api/search?q=rah"

curl -X POST http://localhost:8080/api/tickets \
  -H "Content-Type: application/json" \
  -d '{"name":"Rahim Uddin","status":"done","problem":"Email access issue","assigned_to":"IT Support"}'

curl -X POST http://localhost:8080/api/tickets/TKT-20260410-4821/done

curl "http://localhost:8080/api/tickets?status=pending"
```

(`APP_USER/APP_PASS` চালু থাকলে curl-এ `-u admin:password` যোগ করুন।)

---

## ⚙️ কীভাবে কাজ করে

- **Ticket number:** `TKT-000001`, `TKT-000002`, … — assigned in increasing order by a database counter. Existing ticket numbers are retained.
- **ইমেইল:** শুধু `done` হলে, **DB-তে সেভ হওয়ার পরে**, goroutine-এ পাঠায় — API সাথে সাথে রেসপন্স দেয়। ব্যর্থ হলে ৩ বার চেষ্টা করে; তবুও না গেলে টিকিট সেভ থাকে, শুধু লগে এরর। সফল হলে `email_sent_at` আপডেট হয় আর UI-তে "📧 ইমেইল পাঠানো হয়েছে" দেখায়।
- **ডাবল ক্লিক নিরাপদ:** একই পেন্ডিং টিকিটে একসাথে একাধিকবার Done চাপলেও ইমেইল একবারই যায়।
- **পেন্ডিং তালিকা:** হোম পেজের নিচে "⏳ পেন্ডিং টিকিট" খুলে পরে যেকোনো সময় Done করা যায়।
- **বন্ধ হওয়ার সময়:** চলমান ইমেইলগুলো শেষ হওয়া পর্যন্ত অপেক্ষা করে (সর্বোচ্চ ২৫ সেকেন্ড)।

---

## 🛠️ সমস্যা সমাধান

| সমস্যা | সমাধান |
|---|---|
| `Excel পড়া যায়নি` | `employees.xlsx` প্রজেক্ট ফোল্ডারে আছে কিনা দেখুন, অথবা `go run . -sample-excel` |
| লোকালে ইমেইল যাচ্ছে না | লগ দেখুন। `535 Authentication failed` = SMTP_USER/SMTP_PASS ভুল (API key নয়, SMTP key দিন)। Sender verify করা আছে কিনা দেখুন। |
| Render-এ ইমেইলে `timeout` | `SMTP_PORT=2525` দিয়েছেন কিনা দেখুন |
| মেইল spam-এ যাচ্ছে | Brevo-তে নিজের ডোমেইন authenticate করুন (SPF/DKIM) |
| ফোন নম্বরের শুরুর 0 নেই | Phone কলাম Text ফরম্যাটে করুন |
| UI-তে বাংলা ফন্ট আসছে না | ইন্টারনেট লাগে (Google Fonts + Tailwind CDN) |
| Neon কানেকশন এরর | স্ট্রিং-এ `sslmode=require` আছে কিনা দেখুন |

---

## 🔧 সব Environment Variable

| ভেরিয়েবল | ডিফল্ট | কাজ |
|---|---|---|
| `PORT` | `8080` | সার্ভার পোর্ট (Render নিজে দেয়) |
| `DATABASE_URL` | — | থাকলে PostgreSQL, না থাকলে SQLite |
| `SQLITE_PATH` | `tickets.db` | লোকাল SQLite ফাইল |
| `EXCEL_PATH` | `employees.xlsx` | Employee ফাইল |
| `SMTP_HOST` / `SMTP_PORT` | — / `587` | Brevo: `smtp-relay.brevo.com`, Render-এ `2525` |
| `SMTP_USER` / `SMTP_PASS` | — | Brevo login ও SMTP key |
| `SMTP_FROM` / `SMTP_FROM_NAME` | `SMTP_USER` / `Auto Ticket System` | প্রেরক |
| `GROQ_API_KEY` | — | Chat assistant-এর server-side Groq key |
| `GROQ_MODEL` | `openai/gpt-oss-20b` | Ticket তথ্য extraction model |
| `APP_USER` / `APP_PASS` | — | দুটোই দিলে পুরো সাইটে লগইন |
| `APP_TIMEZONE` | `Asia/Dhaka` | টিকিট নম্বরের তারিখ ও ইমেইলের সময় |
| `GIN_MODE` | `debug` | প্রোডাকশনে `release` |


### Employee data source

The real `employees.xlsx` workbook is excluded from Git because it contains employee identifiers and contact details. On a fresh clone, run `go run . -sample-excel` to create synthetic demo data. For real use, provide the workbook through a private local or deployment path and set `EXCEL_PATH` accordingly. The loader reads the `Master File` worksheet and uses official `DS Mail` only; personal email is never used as a fallback.


## Local PostgreSQL with Docker

Docker Desktop must be running. The checked-in `compose.yaml` starts PostgreSQL 16 on host port `55432`, with persistent data in a named Docker volume. This port is separate from the usual PostgreSQL ports.

1. Copy `.env.example` to `.env`. The example is configured for the local Docker database.
2. Start PostgreSQL: `docker compose -p auto-ticket-system up -d --wait postgres`
3. Start the app from this folder: `go run .`
4. Open `http://localhost:18080`. Check `http://localhost:18080/healthz` for the app and database status.

The local database credentials in `.env.example` are for development only. Ticket records persist when the container restarts. Stop PostgreSQL with `docker compose -p auto-ticket-system stop postgres`; keep the named volume to preserve data.

## Application architecture

The application follows a layered flow while keeping `go run .` as the local entry point:

```mermaid
flowchart LR
    Browser --> HTTP[HTTP adapter]
    HTTP --> UseCase[Ticket use case]
    UseCase --> Domain[Domain models and repository port]
    Postgres[PostgreSQL / SQLite adapter] -.implements.-> Domain
    Excel[Excel employee adapter] -.implements.-> UseCase
    SMTP[SMTP email adapter] -.implements.-> UseCase
    Main[main.go] --> App[Application composition root]
    App --> Config[Environment config]
    App --> HTTP
    App --> UseCase
    App --> Postgres
    App --> Excel
    App --> SMTP
```

Business rules and ticket lifecycle live in the use case layer. The PostgreSQL/SQLite, Excel, SMTP, and Gin integrations sit in adapters, so those libraries do not define the business flow.

The Admin Panel is available at `/admin`. Ticket creation accepts a required problem description and optional assignee. In the Admin Panel, the description is visible and the assignee can be changed or cleared. Both fields are saved in PostgreSQL.
