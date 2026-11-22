#!/bin/bash
# 真实跑通 Keycloak 25.0 的授权码 + PKCE 流程（headless）
set -u
KC=http://localhost:18080
J=/tmp/luban-verify/out/49.2-kc-cookies.txt
rm -f "$J"

echo "== 0) 版本"
docker exec kc /opt/keycloak/bin/kc.sh --version 2>/dev/null | tail -2

echo "== 1) admin token"
ADMIN=$(curl -s -d "client_id=admin-cli" -d "username=admin" -d "password=admin" -d "grant_type=password" "$KC/realms/master/protocol/openid-connect/token" | jq -r .access_token)
echo "admin_token_len=${#ADMIN}"

echo "== 2) 建 realm prod / client billing / user alice"
curl -s -o /dev/null -w "delete_realm=%{http_code}\n" -X DELETE "$KC/admin/realms/prod" -H "Authorization: Bearer $ADMIN"
curl -s -o /dev/null -w "create_realm=%{http_code}\n" -X POST "$KC/admin/realms" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"realm":"prod","enabled":true}'
curl -s -o /dev/null -w "create_client=%{http_code}\n" -X POST "$KC/admin/realms/prod/clients" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"clientId":"billing","publicClient":true,"redirectUris":["https://billing.example.com/callback"],"attributes":{"pkce.code.challenge.method":"S256"}}'
curl -s -o /dev/null -w "create_user=%{http_code}\n" -X POST "$KC/admin/realms/prod/users" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"username":"alice","enabled":true,"firstName":"Alice","lastName":"Liddell","email":"alice@example.com","emailVerified":true,"credentials":[{"type":"password","value":"pass123","temporary":false}]}'

echo "== 3) discovery（prod realm）"
curl -s "$KC/realms/prod/.well-known/openid-configuration" | jq -c '{issuer,authorization_endpoint,token_endpoint,jwks_uri,code_challenge_methods_supported}'

echo "== 4) 生成 PKCE verifier/challenge"
VERIFIER=$(openssl rand -base64 48 | tr -d '=+/' | cut -c1-64)
CHALLENGE=$(printf %s "$VERIFIER" | openssl dgst -binary -sha256 | openssl base64 | tr -d '=' | tr '/+' '_-')
echo "verifier_len=${#VERIFIER} challenge=${CHALLENGE:0:16}..."

echo "== 5) GET /auth 取登录页表单"
AUTH_URL="$KC/realms/prod/protocol/openid-connect/auth?response_type=code&client_id=billing&redirect_uri=https%3A%2F%2Fbilling.example.com%2Fcallback&scope=openid&state=st-42&code_challenge=$CHALLENGE&code_challenge_method=S256"
LOGIN_HTML=$(curl -s -c "$J" -b "$J" "$AUTH_URL")
ACTION=$(printf '%s' "$LOGIN_HTML" | grep -o 'action="[^"]*"' | head -1 | sed 's/action="//;s/"$//' | sed 's/&amp;/\&/g')
echo "action=$ACTION"

echo "== 6) POST 用户名密码"
LOC=$(curl -s -o /dev/null -D - -c "$J" -b "$J" -X POST "$ACTION" \
  --data-urlencode "username=alice" --data-urlencode "password=pass123" --data-urlencode "credentialId=" | grep -i '^location:' | tr -d '\r')
echo "$LOC"
CODE=$(printf '%s' "$LOC" | grep -o 'code=[^&]*' | head -1 | cut -d= -f2)
echo "code_len=${#CODE}"

echo "== 7) 用 code 换 token（带 code_verifier）"
TOK=$(curl -s -X POST "$KC/realms/prod/protocol/openid-connect/token" \
  -d "grant_type=authorization_code" -d "code=$CODE" -d "client_id=billing" \
  -d "redirect_uri=https://billing.example.com/callback" -d "code_verifier=$VERIFIER")
echo "$TOK" | jq -c '{token_type,expires_in,has_id_token:(.id_token!=null),has_access_token:(.access_token!=null)}'
echo "== 8) id_token 头/载荷"
printf '%s' "$TOK" | jq -r .id_token | python3 -c 'import base64,json,sys
t=sys.stdin.read().strip()
for p in t.split(".")[:2]:
    print(json.dumps(json.loads(base64.urlsafe_b64decode(p+"="*(-len(p)%4))),ensure_ascii=False))'
echo "== 9) 重复用同一 code（应失败）"
curl -s -w "\nhttp=%{http_code}\n" -X POST "$KC/realms/prod/protocol/openid-connect/token" \
  -d "grant_type=authorization_code" -d "code=$CODE" -d "client_id=billing" \
  -d "redirect_uri=https://billing.example.com/callback" -d "code_verifier=$VERIFIER" | head -c 400
