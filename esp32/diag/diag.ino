// Diagnostic v8: WiFi + periodic HTTP poll to aggregator.
// Tests sustained connectivity. NeoPixel shows status:
//   BLUE  = connecting WiFi
//   GREEN = WiFi OK, polling
//   RED   = WiFi or HTTP failed
// Prints all status to Serial.

#include <WiFi.h>
#include <HTTPClient.h>

const char *WIFI_SSID = "13r";
const char *WIFI_PASS = "ballball";
const char *AGG_HOST  = "10.201.115.150";
const int   AGG_PORT  = 9091;

uint32_t pollCount = 0;
uint32_t okCount   = 0;
uint32_t failCount = 0;

void setup() {
    Serial.begin(115200);
    delay(500);
    Serial.println("\n=== DIAG v8: WiFi + HTTP ===");

    pinMode(8, OUTPUT);
    rgbLedWrite(8, 0, 0, 50);  // blue = connecting

    WiFi.mode(WIFI_STA);
    WiFi.begin(WIFI_SSID, WIFI_PASS);
    Serial.printf("Connecting to %s ", WIFI_SSID);

    int tries = 0;
    while (WiFi.status() != WL_CONNECTED && tries < 40) {
        delay(500);
        Serial.print(".");
        tries++;
    }

    if (WiFi.status() != WL_CONNECTED) {
        Serial.println("\nWiFi FAILED");
        rgbLedWrite(8, 50, 0, 0);  // red
        while (true) delay(1000);
    }

    Serial.printf("\nWiFi OK: %s\n", WiFi.localIP().toString().c_str());
    rgbLedWrite(8, 0, 50, 0);  // green
}

void loop() {
    // Reconnect if needed
    if (WiFi.status() != WL_CONNECTED) {
        Serial.println("[WIFI] lost, reconnecting...");
        rgbLedWrite(8, 50, 50, 0);  // yellow
        WiFi.reconnect();
        int t = 0;
        while (WiFi.status() != WL_CONNECTED && t < 20) {
            delay(500);
            t++;
        }
        if (WiFi.status() != WL_CONNECTED) {
            Serial.println("[WIFI] reconnect failed");
            rgbLedWrite(8, 50, 0, 0);
            delay(5000);
            return;
        }
        Serial.println("[WIFI] reconnected");
        rgbLedWrite(8, 0, 50, 0);
    }

    pollCount++;
    char url[128];
    snprintf(url, sizeof(url), "http://%s:%d/api/defense", AGG_HOST, AGG_PORT);

    HTTPClient http;
    http.begin(url);
    http.setTimeout(5000);
    int code = http.GET();

    if (code == 200) {
        okCount++;
        String body = http.getString();
        Serial.printf("[%u] HTTP 200 OK  (ok=%u fail=%u)  body=%s\n",
                      pollCount, okCount, failCount, body.c_str());
        rgbLedWrite(8, 0, 50, 0);  // green
    } else {
        failCount++;
        Serial.printf("[%u] HTTP FAIL code=%d err=%s  (ok=%u fail=%u)\n",
                      pollCount, code, http.errorToString(code).c_str(),
                      okCount, failCount);
        rgbLedWrite(8, 50, 0, 0);  // red
    }

    http.end();
    delay(3000);  // poll every 3 seconds
}
