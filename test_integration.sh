#!/bin/bash
set -uo pipefail

KV=/tmp/kint-vault
BASE=/tmp/kv-integration
PASS=0
FAIL=0
export NO_COLOR=1

pass() { PASS=$((PASS+1)); echo "    PASS  $1"; }
fail() { FAIL=$((FAIL+1)); echo "    FAIL  $1 — $2"; }

fresh_project() {
  rm -rf "$BASE" && mkdir -p "$BASE" && cd "$BASE"
  echo "dev" | $KV init --env dev >/dev/null 2>&1
  printf 'API_KEY=sk-secret-123\nDB_HOST=localhost\nDB_PASS=p@ssw0rd\n' > .env
  $KV push -y >/dev/null 2>&1
  rm .env
}

# ============================================================
echo ""
echo "=== SMOKE ==="
echo ""
# ============================================================

fresh_project

# S1
OUT=$($KV pull --json 2>&1)
echo "$OUT" | grep -q '"API_KEY": "sk-secret-123"' && pass "S1 pull roundtrip" || fail "S1 pull roundtrip" "$OUT"

# S2
$KV set X=1 >/dev/null 2>&1
OUT=$($KV get X 2>&1)
[ "$OUT" = "1" ] && pass "S2 set+get" || fail "S2 set+get" "$OUT"

# S3
$KV delete X -y >/dev/null 2>&1
OUT=$($KV get X 2>&1 || true)
echo "$OUT" | grep -qi "not found" && pass "S3 delete" || fail "S3 delete" "$OUT"

# S4
OUT=$($KV doctor 2>&1)
echo "$OUT" | grep -q "Decryption works" && pass "S4 doctor" || fail "S4 doctor" "$OUT"

# S5
OUT=$($KV run -- sh -c 'echo $API_KEY' 2>&1)
[ "$OUT" = "sk-secret-123" ] && pass "S5 run" || fail "S5 run" "$OUT"

# ============================================================
echo ""
echo "=== HAPPY PATH: INIT ==="
echo ""
# ============================================================

rm -rf "$BASE" && mkdir -p "$BASE" && cd "$BASE"

# H1
echo "dev" | $KV init --env dev >/dev/null 2>&1
[ -f .kint-vault.yaml ] && [ -f .gitignore ] && pass "H1 init creates files" || fail "H1 init creates files" "missing"

# H2
grep -q "dev" .kint-vault.yaml && pass "H2 init sets env" || fail "H2 init sets env" ""

# H3
grep -q ".env" .gitignore && pass "H3 init adds .gitignore" || fail "H3 init adds .gitignore" ""

# H4
echo "staging" | $KV init --env staging --force >/dev/null 2>&1
grep -q "staging" .kint-vault.yaml && pass "H4 init --force" || fail "H4 init --force" ""
echo "dev" | $KV init --env dev --force >/dev/null 2>&1

# ============================================================
echo ""
echo "=== HAPPY PATH: PUSH / PULL ==="
echo ""
# ============================================================

fresh_project

# H5
[ -f .env.dev.enc ] && pass "H5 push creates enc" || fail "H5 push creates enc" ""

# H6
$KV pull --force >/dev/null 2>&1
[ -f .env ] && pass "H6 pull creates .env" || fail "H6 pull creates .env" ""

# H7
grep -q "sk-secret-123" .env && grep -q "localhost" .env && grep -q "p@ssw0rd" .env && pass "H7 pull content correct" || fail "H7 pull content correct" ""

# H8
rm .env
OUT=$($KV pull --json 2>&1)
echo "$OUT" | python3 -c "import sys,json; d=json.load(sys.stdin); assert len(d)==3" 2>/dev/null && pass "H8 pull --json" || fail "H8 pull --json" "$OUT"

# H9
OUT=$($KV pull --stdout 2>&1)
echo "$OUT" | grep -q "API_KEY=sk-secret-123" && pass "H9 pull --stdout" || fail "H9 pull --stdout" "$OUT"

# H10
rm -f .env.custom
$KV pull -o .env.custom >/dev/null 2>&1
[ -f .env.custom ] && grep -q "sk-secret-123" .env.custom && pass "H10 pull -o" || fail "H10 pull -o" ""
rm -f .env.custom

# H11
rm -f .env
$KV pull --env dev --force >/dev/null 2>&1
grep -q "sk-secret-123" .env && pass "H11 pull --env" || fail "H11 pull --env" ""

# H12 push -f custom file
printf 'CUSTOM=val\n' > .env.staging
$KV env staging >/dev/null 2>&1
$KV push -y -f .env.staging >/dev/null 2>&1
[ -f .env.staging.enc ] && pass "H12 push -f custom" || fail "H12 push -f custom" ""
$KV env dev >/dev/null 2>&1
rm -f .env.staging .env.staging.enc

# ============================================================
echo ""
echo "=== HAPPY PATH: SET / GET / DELETE ==="
echo ""
# ============================================================

fresh_project

# H13
$KV set A=1 B=2 C=3 >/dev/null 2>&1
OA=$($KV get A 2>&1); OB=$($KV get B 2>&1); OC=$($KV get C 2>&1)
[ "$OA" = "1" ] && [ "$OB" = "2" ] && [ "$OC" = "3" ] && pass "H13 set multiple" || fail "H13 set multiple" "A=$OA B=$OB C=$OC"

# H14
$KV set A=updated >/dev/null 2>&1
OUT=$($KV get A 2>&1)
[ "$OUT" = "updated" ] && pass "H14 set overwrite" || fail "H14 set overwrite" "$OUT"

# H15
$KV delete A B -y >/dev/null 2>&1
OUT=$($KV list 2>&1)
echo "$OUT" | grep -q "^A$" && fail "H15 delete multiple" "A still exists" || pass "H15 delete multiple"

# H16
$KV delete C -y >/dev/null 2>&1
OUT=$($KV list 2>&1)
echo "$OUT" | grep -q "C" && fail "H16 delete last added" "C still exists" || pass "H16 delete last added"

# ============================================================
echo ""
echo "=== HAPPY PATH: LIST ==="
echo ""
# ============================================================

fresh_project

# H17
OUT=$($KV list 2>&1)
LINES=$(echo "$OUT" | wc -l | tr -d ' ')
[ "$LINES" = "3" ] && pass "H17 list count" || fail "H17 list count" "expected 3, got $LINES"

# H18
OUT=$($KV list --json 2>&1)
echo "$OUT" | python3 -c "import sys,json; d=json.load(sys.stdin); assert isinstance(d,list) and len(d)==3" 2>/dev/null && pass "H18 list --json" || fail "H18 list --json" "$OUT"

# ============================================================
echo ""
echo "=== HAPPY PATH: DIFF ==="
echo ""
# ============================================================

fresh_project
$KV pull --force >/dev/null 2>&1

# H19
OUT=$($KV diff 2>&1)
echo "$OUT" | grep -q "No differences" && pass "H19 diff no changes" || fail "H19 diff no changes" "$OUT"

# H20
echo "LOCAL_ONLY=x" >> .env
OUT=$($KV diff 2>&1)
echo "$OUT" | grep -q "LOCAL_ONLY" && pass "H20 diff local only" || fail "H20 diff local only" "$OUT"

# H21
$KV set REMOTE_ONLY=y >/dev/null 2>&1
OUT=$($KV diff 2>&1)
echo "$OUT" | grep -q "REMOTE_ONLY" && pass "H21 diff remote only" || fail "H21 diff remote only" "$OUT"

# H22
printf 'API_KEY=changed\nDB_HOST=localhost\nDB_PASS=p@ssw0rd\n' > .env
OUT=$($KV diff 2>&1)
echo "$OUT" | grep -q "API_KEY" && pass "H22 diff modified" || fail "H22 diff modified" "$OUT"

# H23
OUT=$($KV diff --json 2>&1)
echo "$OUT" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'added' in d and 'removed' in d and 'modified' in d" 2>/dev/null && pass "H23 diff --json" || fail "H23 diff --json" "$OUT"
$KV delete REMOTE_ONLY -y >/dev/null 2>&1

# ============================================================
echo ""
echo "=== HAPPY PATH: ENV ==="
echo ""
# ============================================================

fresh_project

# H24
OUT=$($KV env 2>&1)
[ "$OUT" = "dev" ] && pass "H24 env show" || fail "H24 env show" "$OUT"

# H25
$KV env production >/dev/null 2>&1
OUT=$($KV env 2>&1)
[ "$OUT" = "production" ] && pass "H25 env switch" || fail "H25 env switch" "$OUT"
$KV env dev >/dev/null 2>&1

# ============================================================
echo ""
echo "=== HAPPY PATH: ROTATE ==="
echo ""
# ============================================================

fresh_project

# H26
cp .env.dev.enc .env.dev.enc.pre
$KV rotate >/dev/null 2>&1
! diff -q .env.dev.enc .env.dev.enc.pre >/dev/null 2>&1 && pass "H26 rotate changes file" || fail "H26 rotate changes file" ""

# H27
OUT=$($KV pull --json 2>&1)
echo "$OUT" | grep -q "sk-secret-123" && pass "H27 rotate preserves values" || fail "H27 rotate preserves values" "$OUT"
rm .env.dev.enc.pre

# ============================================================
echo ""
echo "=== HAPPY PATH: VALIDATE ==="
echo ""
# ============================================================

fresh_project

# H28
printf 'API_KEY=\nDB_HOST=\nDB_PASS=\n' > .env.example
OUT=$($KV validate 2>&1)
echo "$OUT" | grep -qi "3 keys present\|OK" && pass "H28 validate pass" || fail "H28 validate pass" "$OUT"

# H29
printf 'API_KEY=\nDB_HOST=\nDB_PASS=\nMISSING=\n' > .env.example
OUT=$($KV validate 2>&1 || true)
echo "$OUT" | grep -q "MISSING" && pass "H29 validate missing" || fail "H29 validate missing" "$OUT"

# H30
printf 'API_KEY=\n' > .env.example
OUT=$($KV validate --strict 2>&1 || true)
echo "$OUT" | grep -q "DB_HOST\|extra" && pass "H30 validate --strict" || fail "H30 validate --strict" "$OUT"

# H31
OUT=$($KV validate --json 2>&1 || true)
echo "$OUT" | grep -q '"missing"' && pass "H31 validate --json" || fail "H31 validate --json" "$OUT"

# H32
printf 'API_KEY=\nDB_HOST=\nDB_PASS=\n' > .env.example
OUT=$($KV validate -t .env.example 2>&1)
echo "$OUT" | grep -qi "3 keys present\|OK" && pass "H32 validate -t custom" || fail "H32 validate -t custom" "$OUT"
rm .env.example

# ============================================================
echo ""
echo "=== HAPPY PATH: RUN ==="
echo ""
# ============================================================

fresh_project

# H33
OUT=$($KV run -- sh -c 'echo $DB_HOST' 2>&1)
[ "$OUT" = "localhost" ] && pass "H33 run multi-var" || fail "H33 run multi-var" "$OUT"

# H34
$KV run -- sh -c 'exit 42' 2>&1; RC=$?
[ "$RC" = "42" ] && pass "H34 run exit code" || fail "H34 run exit code" "got $RC"

# H35
OUT=$($KV run --env dev -- sh -c 'echo $API_KEY' 2>&1)
[ "$OUT" = "sk-secret-123" ] && pass "H35 run --env" || fail "H35 run --env" "$OUT"

# ============================================================
echo ""
echo "=== HAPPY PATH: DOCTOR ==="
echo ""
# ============================================================

fresh_project

# H36
OUT=$($KV doctor 2>&1)
echo "$OUT" | grep -q "Config file" && echo "$OUT" | grep -q "age key" && echo "$OUT" | grep -q "Decryption works" && pass "H36 doctor all checks" || fail "H36 doctor all checks" "$OUT"

# ============================================================
echo ""
echo "=== HAPPY PATH: FORMAT ==="
echo ""
# ============================================================

fresh_project

# H37
grep -q "type:str" .env.dev.enc && pass "H37 type tag" || fail "H37 type tag" ""

# H38
grep -q "kv_mac=ENC\[" .env.dev.enc && pass "H38 encrypted MAC" || fail "H38 encrypted MAC" ""

# H39
grep -q "kv_version=1" .env.dev.enc && pass "H39 version field" || fail "H39 version field" ""

# H40
grep -q "kv_age_recipient_0=" .env.dev.enc && pass "H40 kv_ prefix recipients" || fail "H40 kv_ prefix recipients" ""

# H41
grep -q "kv_age_key=" .env.dev.enc && pass "H41 kv_ prefix age key" || fail "H41 kv_ prefix age key" ""

# H42
! grep -q "sops_" .env.dev.enc && pass "H42 no sops_ prefix" || fail "H42 no sops_ prefix" "found sops_"

# ============================================================
echo ""
echo "=== HAPPY PATH: SOPS_AGE_KEY ==="
echo ""
# ============================================================

fresh_project

# H43
AGE_KEY_FILE="$HOME/Library/Application Support/sops/age/keys.txt"
[ ! -f "$AGE_KEY_FILE" ] && AGE_KEY_FILE="$HOME/.config/sops/age/keys.txt"
KEY_CONTENT=$(cat "$AGE_KEY_FILE")
OUT=$(SOPS_AGE_KEY="$KEY_CONTENT" $KV pull --json 2>&1)
echo "$OUT" | grep -q "sk-secret-123" && pass "H43 SOPS_AGE_KEY" || fail "H43 SOPS_AGE_KEY" "$OUT"

# ============================================================
echo ""
echo "=== ERROR PATHS ==="
echo ""
# ============================================================

# E1
cd /tmp && rm -rf kv-err-noconfig && mkdir kv-err-noconfig && cd kv-err-noconfig
OUT=$($KV pull 2>&1 || true)
echo "$OUT" | grep -qi "no .kint-vault.yaml\|not found" && pass "E1 no config" || fail "E1 no config" "$OUT"

# E2
fresh_project
OUT=$($KV init --env dev 2>&1 || true)
echo "$OUT" | grep -qi "already exists" && pass "E2 init exists" || fail "E2 init exists" "$OUT"

# E3
OUT=$($KV push -y -f nonexistent.env 2>&1 || true)
echo "$OUT" | grep -qi "not found\|no such" && pass "E3 push missing file" || fail "E3 push missing file" "$OUT"

# E4
$KV env staging >/dev/null 2>&1
OUT=$($KV pull 2>&1 || true)
echo "$OUT" | grep -qi "no encrypted" && pass "E4 pull missing enc" || fail "E4 pull missing enc" "$OUT"
$KV env dev >/dev/null 2>&1

# E5
OUT=$($KV get NONEXISTENT 2>&1 || true)
echo "$OUT" | grep -qi "not found" && pass "E5 get missing key" || fail "E5 get missing key" "$OUT"

# E6
OUT=$($KV delete NONEXISTENT -y 2>&1 || true)
echo "$OUT" | grep -qi "not found" && pass "E6 delete missing key" || fail "E6 delete missing key" "$OUT"

# E7
OUT=$($KV set NOEQ 2>&1 || true)
echo "$OUT" | grep -qi "invalid format\|KEY=VALUE" && pass "E7 set no equals" || fail "E7 set no equals" "$OUT"

# E8
OUT=$($KV run 2>&1 || true)
echo "$OUT" | grep -qi "no command" && pass "E8 run no command" || fail "E8 run no command" "$OUT"

# E9
OUT=$($KV env '../../../etc/passwd' 2>&1 || true)
echo "$OUT" | grep -qi "invalid" && pass "E9 invalid env name" || fail "E9 invalid env name" "$OUT"

# E10
OUT=$($KV add-recipient "not-age-key" 2>&1 || true)
echo "$OUT" | grep -qi "invalid\|age1" && pass "E10 invalid recipient" || fail "E10 invalid recipient" "$OUT"

# E11
rm -f .env.example
OUT=$($KV validate 2>&1 || true)
echo "$OUT" | grep -qi "template not found\|.env.example" && pass "E11 validate no template" || fail "E11 validate no template" "$OUT"

# E12
OUT=$($KV set 'kv_test=bad' 2>&1 || true)
echo "$OUT" | grep -qi 'reserved prefix' && pass "E12 kv_ prefix rejected set" || fail "E12 kv_ prefix rejected set" "$OUT"

# E13
printf 'kv_evil=1\n' > .env
OUT=$($KV push -y 2>&1 || true)
echo "$OUT" | grep -qi 'reserved prefix' && pass "E13 kv_ prefix rejected push" || fail "E13 kv_ prefix rejected push" "$OUT"
rm -f .env

# E14 empty env name shows current (no switch)
OUT=$($KV env '' 2>&1 || true)
[ "$OUT" = "dev" ] && pass "E14 empty env shows current" || fail "E14 empty env shows current" "$OUT"

rm -rf /tmp/kv-err-noconfig

# ============================================================
echo ""
echo "=== EDGE CASES ==="
echo ""
# ============================================================

fresh_project

# EC1 empty value
$KV set EMPTY= >/dev/null 2>&1
OUT=$($KV get EMPTY 2>&1)
[ "$OUT" = "" ] && pass "EC1 empty value" || fail "EC1 empty value" "got '$OUT'"

# EC2 value with spaces
$KV set SPACES="hello world foo" >/dev/null 2>&1
OUT=$($KV get SPACES 2>&1)
[ "$OUT" = "hello world foo" ] && pass "EC2 value with spaces" || fail "EC2 value with spaces" "$OUT"

# EC3 value with equals
$KV set URL="postgres://h:5432/db?opt=1&x=2" >/dev/null 2>&1
OUT=$($KV get URL 2>&1)
[ "$OUT" = "postgres://h:5432/db?opt=1&x=2" ] && pass "EC3 value with equals" || fail "EC3 value with equals" "$OUT"

# EC4 value with special chars
$KV set 'SPECIAL=p@$$w0rd!#%^&*()' >/dev/null 2>&1
OUT=$($KV get SPECIAL 2>&1)
[ "$OUT" = 'p@$$w0rd!#%^&*()' ] && pass "EC4 special chars" || fail "EC4 special chars" "$OUT"

# EC5 unicode
$KV set 'UNI=héllo wörld 日本語 🔑' >/dev/null 2>&1
OUT=$($KV get UNI 2>&1)
[ "$OUT" = 'héllo wörld 日本語 🔑' ] && pass "EC5 unicode" || fail "EC5 unicode" "$OUT"

# EC6 long key
LONGKEY=$(python3 -c "print('K'*200)")
$KV set "${LONGKEY}=longval" >/dev/null 2>&1
OUT=$($KV get "$LONGKEY" 2>&1)
[ "$OUT" = "longval" ] && pass "EC6 200-char key" || fail "EC6 200-char key" "$OUT"

# EC7 long value
LONGVAL=$(python3 -c "print('x'*10000)")
$KV set "BIG=${LONGVAL}" >/dev/null 2>&1
OUT=$($KV get BIG 2>&1)
[ "${#OUT}" = "10000" ] && pass "EC7 10KB value" || fail "EC7 10KB value" "len=${#OUT}"

# EC8 duplicate keys in .env (last wins)
printf 'DUP=first\nDUP=second\n' > .env
$KV push -y >/dev/null 2>&1
OUT=$($KV get DUP 2>&1)
[ "$OUT" = "second" ] && pass "EC8 duplicate keys last wins" || fail "EC8 duplicate keys last wins" "$OUT"

# EC9 value that looks like ENC[...]
$KV set 'TRICKY=ENC[AES256_GCM,data:fake,iv:fake,tag:fake,type:str]' >/dev/null 2>&1
OUT=$($KV get TRICKY 2>&1)
[ "$OUT" = 'ENC[AES256_GCM,data:fake,iv:fake,tag:fake,type:str]' ] && pass "EC9 ENC-like value" || fail "EC9 ENC-like value" "$OUT"

# EC10 literal \n in value (not newline)
$KV set 'ESCAPED=line1\nline2' >/dev/null 2>&1
OUT=$($KV get ESCAPED 2>&1)
[ "$OUT" = 'line1\nline2' ] && pass "EC10 literal backslash-n" || fail "EC10 literal backslash-n" "$OUT"

# EC11 push empty .env
printf '' > .env
$KV push -y >/dev/null 2>&1
OUT=$($KV pull --json 2>&1)
[ "$OUT" = "{}" ] && pass "EC11 empty .env roundtrip" || fail "EC11 empty .env roundtrip" "$OUT"

# EC12 set on empty vault (first set without push)
rm -rf "$BASE" && mkdir -p "$BASE" && cd "$BASE"
echo "dev" | $KV init --env dev >/dev/null 2>&1
$KV set FIRST=val >/dev/null 2>&1
OUT=$($KV get FIRST 2>&1)
[ "$OUT" = "val" ] && pass "EC12 first set without push" || fail "EC12 first set without push" "$OUT"

# EC13 many secrets
fresh_project
python3 -c "
for i in range(200):
    print(f'KEY_{i:03d}=value_{i}')
" > .env
$KV push -y >/dev/null 2>&1
OUT=$($KV pull --json 2>&1 | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
[ "$OUT" = "200" ] && pass "EC13 200 secrets" || fail "EC13 200 secrets" "count=$OUT"

# cleanup test keys
fresh_project

# ============================================================
echo ""
echo "=== DEEP EDGE CASES: CRYPTO ==="
echo ""
# ============================================================

fresh_project

# D1 AAD: swap encrypted values between keys
cp .env.dev.enc .env.dev.enc.bak
python3 -c "
lines = open('.env.dev.enc').readlines()
v1 = lines[0].split('=', 1)[1]
v2 = lines[1].split('=', 1)[1]
lines[0] = lines[0].split('=', 1)[0] + '=' + v2
lines[1] = lines[1].split('=', 1)[0] + '=' + v1
open('.env.dev.enc', 'w').writelines(lines)
"
OUT=$($KV pull --json 2>&1 || true)
echo "$OUT" | grep -qi "error\|fail" && pass "D1 AAD swap detected" || fail "D1 AAD swap detected" "$OUT"
mv .env.dev.enc.bak .env.dev.enc

# D2 missing MAC
cp .env.dev.enc .env.dev.enc.bak
python3 -c "
lines = [l for l in open('.env.dev.enc').readlines() if not l.startswith('kv_mac=')]
open('.env.dev.enc', 'w').writelines(lines)
"
OUT=$($KV pull --json 2>&1 || true)
echo "$OUT" | grep -qi "error\|fail" && pass "D2 missing MAC" || fail "D2 missing MAC" "$OUT"
mv .env.dev.enc.bak .env.dev.enc

# D3 plaintext injection + MAC removal
cp .env.dev.enc .env.dev.enc.bak
python3 -c "
lines = open('.env.dev.enc').readlines()
lines[0] = 'API_KEY=injected\n'
lines = [l for l in lines if not l.startswith('kv_mac=')]
open('.env.dev.enc', 'w').writelines(lines)
"
OUT=$($KV pull --json 2>&1 || true)
echo "$OUT" | grep -qi "error\|fail" && pass "D3 plaintext + no MAC" || fail "D3 plaintext + no MAC" "$OUT"
mv .env.dev.enc.bak .env.dev.enc

# D4 plaintext value with MAC present
cp .env.dev.enc .env.dev.enc.bak
python3 -c "
lines = open('.env.dev.enc').readlines()
lines[0] = 'API_KEY=plaintext\n'
open('.env.dev.enc', 'w').writelines(lines)
"
OUT=$($KV pull --json 2>&1 || true)
echo "$OUT" | grep -qi "error\|fail" && pass "D4 plaintext value rejected" || fail "D4 plaintext value rejected" "$OUT"
mv .env.dev.enc.bak .env.dev.enc

# D5 corrupted base64 in data field
cp .env.dev.enc .env.dev.enc.bak
python3 -c "
lines = open('.env.dev.enc').readlines()
lines[0] = 'API_KEY=ENC[AES256_GCM,data:!!!corrupt!!!,iv:AAAA,tag:BBBB,type:str]\n'
open('.env.dev.enc', 'w').writelines(lines)
"
OUT=$($KV pull --json 2>&1 || true)
echo "$OUT" | grep -qi "error\|fail" && pass "D5 corrupted base64" || fail "D5 corrupted base64" "$OUT"
mv .env.dev.enc.bak .env.dev.enc

# D6 missing kv_age_key
cp .env.dev.enc .env.dev.enc.bak
python3 -c "
lines = [l for l in open('.env.dev.enc').readlines() if not l.startswith('kv_age_key=')]
open('.env.dev.enc', 'w').writelines(lines)
"
OUT=$($KV pull --json 2>&1 || true)
echo "$OUT" | grep -qi "error\|fail" && pass "D6 missing data key" || fail "D6 missing data key" "$OUT"
mv .env.dev.enc.bak .env.dev.enc

# D7 tampered MAC value
cp .env.dev.enc .env.dev.enc.bak
python3 -c "
lines = open('.env.dev.enc').readlines()
for i, l in enumerate(lines):
    if l.startswith('kv_mac='):
        lines[i] = 'kv_mac=ENC[AES256_GCM,data:AAAA,iv:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=,tag:AAAAAAAAAAAAAAAAAAAAAA==,type:str]\n'
open('.env.dev.enc', 'w').writelines(lines)
"
OUT=$($KV pull --json 2>&1 || true)
echo "$OUT" | grep -qi "error\|fail" && pass "D7 tampered MAC" || fail "D7 tampered MAC" "$OUT"
mv .env.dev.enc.bak .env.dev.enc

# ============================================================
echo ""
echo "=== DEEP EDGE CASES: STASH + DATA KEY REUSE ==="
echo ""
# ============================================================

fresh_project

# D8 no-op set: all secret lines unchanged
cp .env.dev.enc .env.dev.enc.before
$KV set API_KEY=sk-secret-123 >/dev/null 2>&1
BEFORE=$(grep "^API_KEY=" .env.dev.enc.before)
AFTER=$(grep "^API_KEY=" .env.dev.enc)
[ "$BEFORE" = "$AFTER" ] && pass "D8 stash: no-op same ciphertext" || fail "D8 stash: no-op same ciphertext" ""

# D9 all unchanged lines stay identical
B_DB=$(grep "^DB_HOST=" .env.dev.enc.before)
A_DB=$(grep "^DB_HOST=" .env.dev.enc)
B_PASS=$(grep "^DB_PASS=" .env.dev.enc.before)
A_PASS=$(grep "^DB_PASS=" .env.dev.enc)
[ "$B_DB" = "$A_DB" ] && [ "$B_PASS" = "$A_PASS" ] && pass "D9 stash: other keys unchanged" || fail "D9 stash: other keys unchanged" ""
rm .env.dev.enc.before

# D10 change one value: only that line differs
$KV set STASH_A=original >/dev/null 2>&1
cp .env.dev.enc .env.dev.enc.before
$KV set STASH_A=changed >/dev/null 2>&1
B_API=$(grep "^API_KEY=" .env.dev.enc.before)
A_API=$(grep "^API_KEY=" .env.dev.enc)
B_ST=$(grep "^STASH_A=" .env.dev.enc.before)
A_ST=$(grep "^STASH_A=" .env.dev.enc)
[ "$B_API" = "$A_API" ] && [ "$B_ST" != "$A_ST" ] && pass "D10 stash: only changed differs" || fail "D10 stash: only changed differs" ""
$KV delete STASH_A -y >/dev/null 2>&1
rm -f .env.dev.enc.before

# D11 add new key: existing lines unchanged
cp .env.dev.enc .env.dev.enc.before
$KV set BRAND_NEW=fresh >/dev/null 2>&1
B_API=$(grep "^API_KEY=" .env.dev.enc.before)
A_API=$(grep "^API_KEY=" .env.dev.enc)
[ "$B_API" = "$A_API" ] && pass "D11 stash: add key keeps existing" || fail "D11 stash: add key keeps existing" ""
$KV delete BRAND_NEW -y >/dev/null 2>&1
rm -f .env.dev.enc.before

# D12 delete key: remaining lines unchanged
$KV set TEMP=willdelete >/dev/null 2>&1
cp .env.dev.enc .env.dev.enc.before
$KV delete TEMP -y >/dev/null 2>&1
B_API=$(grep "^API_KEY=" .env.dev.enc.before)
A_API=$(grep "^API_KEY=" .env.dev.enc)
[ "$B_API" = "$A_API" ] && pass "D12 stash: delete keeps remaining" || fail "D12 stash: delete keeps remaining" ""
rm -f .env.dev.enc.before

# D13 rotate MUST change all ciphertexts (new data key)
cp .env.dev.enc .env.dev.enc.before
$KV rotate >/dev/null 2>&1
B_API=$(grep "^API_KEY=" .env.dev.enc.before)
A_API=$(grep "^API_KEY=" .env.dev.enc)
[ "$B_API" != "$A_API" ] && pass "D13 rotate changes ciphertext" || fail "D13 rotate changes ciphertext" ""
rm -f .env.dev.enc.before

# ============================================================
echo ""
echo "=== MONOREPO ==="
echo ""
# ============================================================

rm -rf "$BASE" && mkdir -p "$BASE" && cd "$BASE"

# Setup monorepo structure
echo "dev" | $KV init --env dev >/dev/null 2>&1

mkdir -p services/api services/web
printf 'API_PORT=8080\nAPI_SECRET=abc\n' > services/api/.env
printf 'WEB_PORT=3000\nWEB_KEY=xyz\n' > services/web/.env

# M1 push --all
$KV push --all -y >/dev/null 2>&1
[ -f services/api/.env.dev.enc ] && [ -f services/web/.env.dev.enc ] && pass "M1 push --all" || fail "M1 push --all" "missing enc files"

rm services/api/.env services/web/.env

# M2 pull --all
$KV pull --all --force >/dev/null 2>&1
[ -f services/api/.env ] && [ -f services/web/.env ] && pass "M2 pull --all" || fail "M2 pull --all" "missing .env files"
grep -q "API_PORT=8080" services/api/.env && grep -q "WEB_PORT=3000" services/web/.env && pass "M3 pull --all content" || fail "M3 pull --all content" ""

# M4 list --all
OUT=$($KV list --all 2>&1)
echo "$OUT" | grep -q "API_PORT" && echo "$OUT" | grep -q "WEB_PORT" && pass "M4 list --all" || fail "M4 list --all" "$OUT"

# M5 list --all --json
OUT=$($KV list --all --json 2>&1)
echo "$OUT" | python3 -c "import sys,json; d=json.load(sys.stdin); assert len(d)>=2" 2>/dev/null && pass "M5 list --all --json" || fail "M5 list --all --json" "$OUT"

# M6 diff --all
OUT=$($KV diff --all 2>&1)
echo "$OUT" | grep -qi "no differences\|ok" && pass "M6 diff --all no changes" || fail "M6 diff --all no changes" "$OUT"

# M7 diff --all with change
echo "EXTRA=x" >> services/api/.env
OUT=$($KV diff --all 2>&1)
echo "$OUT" | grep -q "EXTRA" && pass "M7 diff --all detects change" || fail "M7 diff --all detects change" "$OUT"

# M8 diff --all --json
OUT=$($KV diff --all --json 2>&1)
echo "$OUT" | python3 -c "import sys,json; d=json.load(sys.stdin); assert len(d)>=1" 2>/dev/null && pass "M8 diff --all --json" || fail "M8 diff --all --json" "$OUT"

# M9 validate --all
printf 'API_PORT=\nAPI_SECRET=\n' > services/api/.env.example
printf 'WEB_PORT=\nWEB_KEY=\n' > services/web/.env.example
OUT=$($KV validate --all 2>&1)
echo "$OUT" | grep -qi "ok\|present" && pass "M9 validate --all" || fail "M9 validate --all" "$OUT"

# M10 validate --all --json
OUT=$($KV validate --all --json 2>&1)
echo "$OUT" | python3 -c "import sys,json; d=json.load(sys.stdin); assert len(d)>=2" 2>/dev/null && pass "M10 validate --all --json" || fail "M10 validate --all --json" "$OUT"

# M11 rotate --all
$KV rotate --all >/dev/null 2>&1
OUT=$($KV pull --all --force 2>&1)
grep -q "API_PORT=8080" services/api/.env && grep -q "WEB_PORT=3000" services/web/.env && pass "M11 rotate --all preserves" || fail "M11 rotate --all preserves" ""

# M12 excluded dirs (node_modules, .venv)
mkdir -p node_modules/pkg .venv/lib
printf 'LEAK=bad\n' > node_modules/pkg/.env
printf 'LEAK2=bad\n' > .venv/lib/.env
$KV push --all -y >/dev/null 2>&1
[ ! -f node_modules/pkg/.env.dev.enc ] && [ ! -f .venv/lib/.env.dev.enc ] && pass "M12 excludes node_modules/.venv" || fail "M12 excludes node_modules/.venv" ""

# ============================================================
echo ""
echo "=== PERMISSION / FILE SECURITY ==="
echo ""
# ============================================================

fresh_project

# P1 pulled .env has restricted permissions
rm -f .env
$KV pull --force >/dev/null 2>&1
PERMS=$(stat -f '%Lp' .env 2>/dev/null || stat -c '%a' .env 2>/dev/null)
[ "$PERMS" = "600" ] && pass "P1 .env has 600 perms" || fail "P1 .env has 600 perms" "got $PERMS"

# ============================================================
echo ""
echo "======================================="
printf "  Results: %d passed, %d failed\n" "$PASS" "$FAIL"
echo "======================================="
echo ""

rm -rf "$BASE"
[ $FAIL -eq 0 ] && exit 0 || exit 1
