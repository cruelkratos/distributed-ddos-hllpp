/*
 * sensor.ino — Arduino Uno as a lightweight IP sensor
 *
 * Receives IPs over serial and forwards them to a Raspberry Pi via USB serial.
 * The Pi runs the full Go agent with HLL++ (p=14, 12KB sketch) for DDoS detection.
 *
 * Serial Protocol (115200 baud):
 *   Commands received:
 *     INSERT <ip>       — Forward IP to Pi
 *     STATS             — Print forwarding stats
 *     RESET             — Reset counters
 *
 *   Responses:
 *     FWD:<ip>          — Forwarded IP (Pi's serial-bridge reads these)
 *     OK                — Command succeeded
 *     ERR:<message>     — Error
 *
 * Memory usage: ~300 bytes of 2048 SRAM (input buffer + counters + stack)
 */

// ─── Counters ────────────────────────────────────────────────────────
static uint32_t forwardCount = 0;

// ─── Serial input buffer ────────────────────────────────────────────
#define BUF_SIZE 64
static char inputBuf[BUF_SIZE];
static uint8_t bufPos = 0;

// =====================================================================
// Command handlers
// =====================================================================

static void cmdInsert(const char *ip) {
    Serial.print(F("FWD:"));
    Serial.println(ip);
    forwardCount++;
}

static void cmdStats(void) {
    Serial.print(F("FORWARDED:"));
    Serial.println(forwardCount);
}

static void cmdReset(void) {
    forwardCount = 0;
    Serial.println(F("OK"));
}

// =====================================================================
// Parse and dispatch a complete line
// =====================================================================
static void processCommand(char *line) {
    // Trim trailing whitespace
    uint8_t len = strlen(line);
    while (len > 0 && (line[len - 1] == '\r' || line[len - 1] == '\n' || line[len - 1] == ' ')) {
        line[--len] = '\0';
    }
    if (len == 0) return;

    // Find first space to split command and argument
    char *space = strchr(line, ' ');
    char *arg = NULL;
    if (space) {
        *space = '\0';
        arg = space + 1;
        while (*arg == ' ') arg++;
    }

    if (strcmp(line, "INSERT") == 0 && arg) {
        cmdInsert(arg);
    } else if (strcmp(line, "STATS") == 0) {
        cmdStats();
    } else if (strcmp(line, "RESET") == 0) {
        cmdReset();
    } else {
        Serial.print(F("ERR:unknown command "));
        Serial.println(line);
    }
}

// =====================================================================
// Arduino setup & loop
// =====================================================================
void setup() {
    Serial.begin(115200);
    while (!Serial) { ; }

    Serial.println(F("READY"));
    Serial.println(F("sensor mode — forwarding IPs to Pi via serial"));
}

void loop() {
    while (Serial.available()) {
        char c = Serial.read();
        if (c == '\n' || c == '\r') {
            if (bufPos > 0) {
                inputBuf[bufPos] = '\0';
                processCommand(inputBuf);
                bufPos = 0;
            }
        } else if (bufPos < BUF_SIZE - 1) {
            inputBuf[bufPos++] = c;
        }
    }
}
