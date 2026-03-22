# WSL2 Validation Checklist

---

## Step 1 — Open PowerShell as Admin

- Hit `Windows key`, type `PowerShell`
- Right-click → **Run as Administrator**

---

## Step 2 — Install WSL2

```powershell
wsl --install
```

- If it asks to reboot → reboot, then come back here

---

## Step 3 — Open Ubuntu

- Hit `Windows key`, type `Ubuntu`, open it
- Wait for it to finish first-time setup (asks for username/password)

---

## Step 4 — Install Go

```bash
sudo apt update && sudo apt install golang-go
```

- Just wait, let it finish

---

## Step 5 — Clone the repo

```bash
git clone <your-repo-url>
cd GPM_Project/gpm
```

---

## Step 6 — Run the integration tests

```bash
go test -tags integration ./internal/adapter/
```

- All tests pass? ✅ Continue
- Something fails? ❌ Screenshot it, message me

---

## Step 7 — Quick sanity check

```bash
echo $PATH
```

- You should see `/mnt/c/...` paths — that's normal for WSL2
- Run GPM and confirm it picks `apt`, not a Windows binary:

```bash
go run . list
```

---

## Step 8 — Done!

---

**Focus tip:** Steps 1–3 are in Windows. Steps 4–8 are inside the Ubuntu terminal. Don't mix them up.
