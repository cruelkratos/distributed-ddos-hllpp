// ESP32-C3 DDoS Detection Edge Agent
// Builds an identical HLL++ sketch (p=14, xxhash64, 6-bit packed registers)
// and ships it to the aggregator over Wi-Fi via HTTP.
// Displays live status on OLED, RGB LED, and buzzer.

#include <WiFi.h>
#include <HTTPClient.h>
#include <WiFiUdp.h>
#include <Wire.h>
#include <Adafruit_GFX.h>
#include <Adafruit_SSD1306.h>
#include "mbedtls/base64.h"
#include "hll.h"

// ===================== CONFIGURATION =====================
// Wi-Fi credentials — MUST be set before flashing.
const char *WIFI_SSID = "13r";
const char *WIFI_PASS = "ballball";

// Aggregator HTTP endpoint (laptop Wi-Fi IP, port 9091).
const char *AGGREGATOR_HOST = "10.201.115.150";
const int   AGGREGATOR_PORT = 9091;

// Node identity reported to the aggregator.
const char *NODE_ID = "esp32-xc3";

// UDP port for receiving IPs from the load-test tool.
const int UDP_PORT = 50052;

// Detection threshold (simple on-device threshold).
const uint64_t ATTACK_THRESHOLD = 2000;

// ===================== PIN CONFIGURATION =================
// Adjust these for your PandaByte xC3 + Grove expansion shield.
#define PIN_SDA       4        // I2C SDA for OLED
#define PIN_SCL       5        // I2C SCL for OLED
#define PIN_NEOPIXEL  6        // Onboard WS2812 RGB LED on GPIO6
#define PIN_BUZZER    3        // Buzzer module (Grove digital port)

// ===================== TIMING ============================
#define WINDOW_MS       10000  // 10 s — matches Go agent window
#define DEFENSE_POLL_MS  5000  // 5 s
#define DISPLAY_MS        500  // 500 ms OLED refresh

// ===================== OLED ==============================
#define SCREEN_W 128
#define SCREEN_H  64
Adafruit_SSD1306 display(SCREEN_W, SCREEN_H, &Wire, -1);

// ===================== STATE =============================
WiFiUDP udp;

uint32_t lastShipTime    = 0;
uint32_t lastDefenseTime = 0;
uint32_t lastDisplayTime = 0;

// Defense state from aggregator.
bool    defenseActivated    = false;
bool    prevDefenseActivated = false;
double  globalScore         = 0.0;
String  defenseReason       = "";

// Buzzer timing (non-blocking).
uint32_t buzzerOffTime = 0;

// Local estimate for display.
uint64_t localCount = 0;

// OLED init status — skip display calls if false.
bool oledReady = false;

// Resource metrics for benchmarking.
uint32_t lastLoopTimeUs = 0;
uint32_t lastShipLatencyMs = 0;
uint32_t loopStartUs = 0;

// ===================== HELPERS ===========================

// Set onboard NeoPixel colour (built-in on ESP32 Arduino core ≥2.0.4).
void setLED(uint8_t r, uint8_t g, uint8_t b) {
    rgbLedWrite(PIN_NEOPIXEL, r, g, b);
}

void beep(int ms) {
    digitalWrite(PIN_BUZZER, HIGH);
    buzzerOffTime = millis() + ms;
}

// ===================== SHIP SKETCH =======================

// Base64-encode the 12 288-byte register array and POST to /api/merge.
void shipSketch() {
    localCount = hll_count();
    int anomalyState = (localCount > ATTACK_THRESHOLD) ? 1 : 0;

    // Simple on-device attack classification.
    const char *attackType = "NONE";
    double attackConfidence = 0.0;
    if (localCount > ATTACK_THRESHOLD) {
        attackType = "UDP_FLOOD";  // ESP32 agent only sees UDP traffic
        attackConfidence = (localCount > ATTACK_THRESHOLD * 3) ? 0.9 : 0.6;
    }

    uint32_t freeHeap = ESP.getFreeHeap();

    // Base64 encode registers.
    static unsigned char b64[16400];
    size_t b64_len = 0;
    int ret = mbedtls_base64_encode(b64, sizeof(b64), &b64_len,
                                    hll_registers, HLL_REG_BYTES);
    if (ret != 0) {
        Serial.printf("[SHIP] base64 encode failed: %d\n", ret);
        return;
    }
    b64[b64_len] = '\0';

    // Build JSON with extended telemetry.
    size_t jsonCap = b64_len + 512;
    char *json = (char *)malloc(jsonCap);
    if (!json) {
        Serial.println("[SHIP] malloc failed");
        return;
    }
    snprintf(json, jsonCap,
             "{\"node_id\":\"%s\",\"p\":%d,\"registers\":\"%s\",\"anomaly_state\":%d,"
             "\"attack_type\":\"%s\",\"attack_confidence\":%.2f,"
             "\"free_heap\":%u,\"ship_latency_ms\":%u,\"loop_time_us\":%u}",
             NODE_ID, HLL_P, (char *)b64, anomalyState,
             attackType, attackConfidence,
             freeHeap, lastShipLatencyMs, lastLoopTimeUs);

    char url[128];
    snprintf(url, sizeof(url),
             "http://%s:%d/api/merge", AGGREGATOR_HOST, AGGREGATOR_PORT);

    uint32_t shipStart = millis();
    HTTPClient http;
    http.begin(url);
    http.addHeader("Content-Type", "application/json");
    http.setTimeout(5000);
    int code = http.POST(json);
    lastShipLatencyMs = millis() - shipStart;

    if (code > 0) {
        Serial.printf("[SHIP] HTTP %d  count=%llu  heap=%u  latency=%ums\n",
                      code, localCount, freeHeap, lastShipLatencyMs);
    } else {
        Serial.printf("[SHIP] FAILED: %s\n",
                      http.errorToString(code).c_str());
    }

    http.end();
    free(json);
}

// ===================== POLL DEFENSE ======================

void pollDefense() {
    char url[128];
    snprintf(url, sizeof(url),
             "http://%s:%d/api/defense", AGGREGATOR_HOST, AGGREGATOR_PORT);

    HTTPClient http;
    http.begin(url);
    http.setTimeout(3000);
    int code = http.GET();

    if (code == 200) {
        String body = http.getString();

        // Simple JSON parse without external lib.
        prevDefenseActivated = defenseActivated;
        defenseActivated = body.indexOf("\"activated\":true") >= 0;

        int idx = body.indexOf("\"global_score\":");
        if (idx >= 0)
            globalScore = body.substring(idx + 15).toDouble();

        int rIdx = body.indexOf("\"reason\":\"");
        if (rIdx >= 0) {
            int rEnd = body.indexOf('"', rIdx + 10);
            defenseReason = body.substring(rIdx + 10, rEnd);
        } else {
            defenseReason = "";
        }

        // Buzzer on defense activation edge.
        if (defenseActivated && !prevDefenseActivated) {
            beep(500);
            Serial.println("[DEFENSE] GLOBAL-DEFENSE ACTIVATED");
        }
    } else {
        Serial.printf("[DEFENSE] poll failed: %d\n", code);
    }
    http.end();
}

// ===================== DISPLAY ===========================

void updateDisplay() {
    if (!oledReady) return;
    display.clearDisplay();
    display.setTextSize(1);     // 6×8 font → 21 chars × 8 lines
    display.setTextColor(SSD1306_WHITE);
    display.setCursor(0, 0);

    display.println("DDoS Edge Agent xC3");
    display.println("--------------------");
    display.printf("Window: %u\n", hll_window_id);
    display.printf("IPs:    %u raw\n", hll_inserts);
    display.printf("HLL:    ~%llu uniq\n", localCount);
    display.println("--------------------");

    if (defenseActivated) {
        display.println("!! GLOBAL DEFENSE !!");
    } else if (localCount > ATTACK_THRESHOLD) {
        display.println("** LOCAL ATTACK **");
    } else {
        display.println("Status: NORMAL");
    }

    display.printf("Score: %.3f", globalScore);
    display.display();
}

void updateLED() {
    if (defenseActivated) {
        // Flashing red: toggle every 100 ms.
        if ((millis() / 100) % 2 == 0)
            setLED(255, 0, 0);
        else
            setLED(0, 0, 0);
    } else if (localCount > ATTACK_THRESHOLD) {
        setLED(255, 0, 0);     // solid red
    } else {
        setLED(0, 255, 0);     // green
    }
}

// ===================== SETUP =============================

void setup() {
    Serial.begin(115200);
    delay(500);
    Serial.println("\n=== ESP32-C3 DDoS Edge Agent ===");

    // I2C + OLED — only init if display is actually connected.
    Wire.begin(PIN_SDA, PIN_SCL);
    oledReady = display.begin(SSD1306_SWITCHCAPVCC, 0x3C);
    if (!oledReady) {
        Wire.end();  // Release SDA/SCL pins so they don't light LEDs
    }
    if (!oledReady) {
        Serial.println("OLED init failed — continuing without display");
    } else {
        display.clearDisplay();
        display.setTextSize(1);
        display.setTextColor(SSD1306_WHITE);
        display.setCursor(0, 0);
        display.println("Connecting WiFi...");
        display.display();
    }

    // NeoPixel + buzzer.
    pinMode(PIN_NEOPIXEL, OUTPUT);
    pinMode(PIN_BUZZER, OUTPUT);
    setLED(0, 0, 255);         // blue while connecting

    // Wi-Fi.
    WiFi.mode(WIFI_STA);
    WiFi.begin(WIFI_SSID, WIFI_PASS);
    Serial.printf("Connecting to %s ", WIFI_SSID);
    int tries = 0;
    while (WiFi.status() != WL_CONNECTED && tries < 60) {
        delay(500);
        Serial.print(".");
        tries++;
    }
    if (WiFi.status() != WL_CONNECTED) {
        Serial.println("\nWiFi FAILED — check SSID/PASS");
        if (oledReady) {
            display.clearDisplay();
            display.setCursor(0, 0);
            display.println("WiFi FAILED!");
            display.println("Check SSID/PASS");
            display.display();
        }
        setLED(255, 255, 0);   // yellow = WiFi error
        while (true) delay(1000);
    }
    Serial.printf("\nWiFi connected: %s\n", WiFi.localIP().toString().c_str());

    if (oledReady) {
        display.clearDisplay();
        display.setCursor(0, 0);
        display.printf("WiFi: %s\n", WiFi.localIP().toString().c_str());
        display.printf("Aggregator:\n %s:%d\n", AGGREGATOR_HOST, AGGREGATOR_PORT);
        display.printf("UDP port: %d\n", UDP_PORT);
        display.display();
        delay(2000);
    }

    // UDP listener.
    udp.begin(UDP_PORT);
    Serial.printf("UDP listening on :%d\n", UDP_PORT);

    // HLL init.
    hll_reset();
    hll_window_id = 0;    // first window is 0
    lastShipTime    = millis();
    lastDefenseTime = millis();
    lastDisplayTime = millis();

    setLED(0, 255, 0);         // green = ready
    Serial.println("Agent ready.");
}

// ===================== LOOP ==============================

void loop() {
    loopStartUs = micros();
    uint32_t now = millis();

    // --- 0. WiFi reconnect ---
    if (WiFi.status() != WL_CONNECTED) {
        Serial.println("[WIFI] disconnected, reconnecting...");
        WiFi.reconnect();
        int tries = 0;
        while (WiFi.status() != WL_CONNECTED && tries < 20) {
            delay(500);
            tries++;
        }
        if (WiFi.status() == WL_CONNECTED) {
            Serial.printf("[WIFI] reconnected: %s\n", WiFi.localIP().toString().c_str());
        } else {
            Serial.println("[WIFI] reconnect failed, will retry next loop");
            return;
        }
    }

    // --- 1. Drain all pending UDP packets ---
    while (true) {
        int pktSize = udp.parsePacket();
        if (pktSize <= 0) break;

        char buf[1500];
        int len = udp.read(buf, sizeof(buf) - 1);
        if (len <= 0) continue;
        buf[len] = '\0';

        // Parse newline-delimited IPs.
        char *saveptr = NULL;
        char *line = strtok_r(buf, "\n", &saveptr);
        while (line) {
            // Trim trailing \r.
            int slen = strlen(line);
            while (slen > 0 && (line[slen - 1] == '\r' || line[slen - 1] == ' '))
                line[--slen] = '\0';
            if (slen >= 7)   // minimum "1.1.1.1"
                hll_insert_ip(line);
            line = strtok_r(NULL, "\n", &saveptr);
        }
    }

    // --- 2. Ship sketch + reset on window boundary ---
    if (now - lastShipTime >= WINDOW_MS) {
        shipSketch();   // always ship so aggregator sees us
        hll_reset();
        lastShipTime = now;
    }

    // --- 3. Poll defense status ---
    if (now - lastDefenseTime >= DEFENSE_POLL_MS) {
        pollDefense();
        lastDefenseTime = now;
    }

    // --- 4. Update display + LED ---
    updateLED();  // called every loop for fast flash rate
    if (now - lastDisplayTime >= DISPLAY_MS) {
        updateDisplay();
        lastDisplayTime = now;
    }

    // --- 5. Non-blocking buzzer off ---
    if (buzzerOffTime > 0 && now >= buzzerOffTime) {
        digitalWrite(PIN_BUZZER, LOW);
        buzzerOffTime = 0;
    }

    // --- 6. WiFi reconnect ---
    if (WiFi.status() != WL_CONNECTED) {
        Serial.println("WiFi lost — reconnecting...");
        setLED(0, 0, 255);
        WiFi.reconnect();
        delay(5000);
    }

    // --- 7. Record loop execution time ---
    lastLoopTimeUs = micros() - loopStartUs;
}
