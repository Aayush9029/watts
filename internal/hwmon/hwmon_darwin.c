#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <IOKit/hidsystem/IOHIDEventSystemClient.h>
#include <mach/mach.h>
#include <math.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>

typedef struct __IOHIDEvent *IOHIDEventRef;
typedef struct __IOHIDServiceClient *IOHIDServiceClientRef;
#ifdef __LP64__
typedef double IOHIDFloat;
#else
typedef float IOHIDFloat;
#endif

IOHIDEventSystemClientRef IOHIDEventSystemClientCreate(CFAllocatorRef allocator);
int IOHIDEventSystemClientSetMatching(IOHIDEventSystemClientRef client, CFDictionaryRef match);
CFArrayRef IOHIDEventSystemClientCopyServices(IOHIDEventSystemClientRef client);
IOHIDEventRef IOHIDServiceClientCopyEvent(IOHIDServiceClientRef service, int64_t type, int32_t options, int64_t timestamp);
CFStringRef IOHIDServiceClientCopyProperty(IOHIDServiceClientRef service, CFStringRef key);
IOHIDFloat IOHIDEventGetFloatValue(IOHIDEventRef event, int32_t field);

#define IOHIDEventFieldBase(type)   (type << 16)
#define kIOHIDEventTypeTemperature  15

typedef struct {
	double avg_temp_c;
	double max_temp_c;
	int temp_sensor_count;
	int fan_count;
	double fan0_rpm;
	double fan1_rpm;
	char error_message[256];
} WattsHWMonSample;

typedef char UInt32Char_t[5];

typedef struct {
	uint8_t major;
	uint8_t minor;
	uint8_t build;
	uint8_t reserved;
	uint16_t release;
} SMCKeyDataVers;

typedef struct {
	uint16_t version;
	uint16_t length;
	uint32_t cpuPLimit;
	uint32_t gpuPLimit;
	uint32_t memPLimit;
} SMCPLimitData;

typedef struct {
	uint32_t dataSize;
	uint32_t dataType;
	uint8_t dataAttributes;
} SMCKeyInfoData;

typedef struct {
	uint32_t key;
	SMCKeyDataVers vers;
	SMCPLimitData pLimitData;
	SMCKeyInfoData keyInfo;
	uint8_t result;
	uint8_t status;
	uint8_t data8;
	uint32_t data32;
	uint8_t bytes[32];
} SMCKeyData;

typedef struct {
	char key[5];
	uint32_t dataSize;
	uint32_t dataType;
	uint8_t bytes[32];
} SMCVal;

enum {
	KERNEL_INDEX_SMC = 2,
	SMC_CMD_READ_BYTES = 5,
	SMC_CMD_READ_KEYINFO = 9
};

static void set_error(WattsHWMonSample *sample, const char *message) {
	if (sample->error_message[0] != '\0') {
		return;
	}
	snprintf(sample->error_message, sizeof(sample->error_message), "%s", message);
}

static bool cfstring_to_cstring(CFStringRef value, char *buffer, size_t size) {
	if (value == NULL || buffer == NULL || size == 0) {
		return false;
	}
	return CFStringGetCString(value, buffer, size, kCFStringEncodingUTF8);
}

static CFDictionaryRef matching_dictionary(int page, int usage) {
	CFStringRef keys[2];
	CFNumberRef values[2];

	keys[0] = CFSTR("PrimaryUsagePage");
	keys[1] = CFSTR("PrimaryUsage");
	values[0] = CFNumberCreate(kCFAllocatorDefault, kCFNumberSInt32Type, &page);
	values[1] = CFNumberCreate(kCFAllocatorDefault, kCFNumberSInt32Type, &usage);

	CFDictionaryRef dict = CFDictionaryCreate(
		kCFAllocatorDefault,
		(const void **)keys,
		(const void **)values,
		2,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);

	CFRelease(values[0]);
	CFRelease(values[1]);
	return dict;
}

static bool is_reasonable_temperature(double value) {
	return isfinite(value) && value >= 0.0 && value < 110.0;
}

static bool is_soc_temperature_sensor(const char *name) {
	if (name == NULL || name[0] == '\0') {
		return false;
	}
	return strstr(name, "tdie") != NULL || strstr(name, "TDIE") != NULL || strstr(name, "SOC MTR Temp") != NULL;
}

static void collect_temperatures(WattsHWMonSample *sample) {
	CFDictionaryRef matching = matching_dictionary(0xff00, 0x0005);
	IOHIDEventSystemClientRef system = IOHIDEventSystemClientCreate(kCFAllocatorDefault);
	if (system == NULL) {
		CFRelease(matching);
		set_error(sample, "create HID event system client");
		return;
	}

	IOHIDEventSystemClientSetMatching(system, matching);
	CFArrayRef services = IOHIDEventSystemClientCopyServices(system);
	CFRelease(matching);
	if (services == NULL) {
		CFRelease(system);
		set_error(sample, "copy HID temperature services");
		return;
	}

	double primary_sum = 0.0;
	double primary_max = 0.0;
	int primary_count = 0;
	double fallback_sum = 0.0;
	double fallback_max = 0.0;
	int fallback_count = 0;

	CFIndex count = CFArrayGetCount(services);
	for (CFIndex i = 0; i < count; i++) {
		IOHIDServiceClientRef service = (IOHIDServiceClientRef)CFArrayGetValueAtIndex(services, i);
		if (service == NULL) {
			continue;
		}

		IOHIDEventRef event = IOHIDServiceClientCopyEvent(service, kIOHIDEventTypeTemperature, 0, 0);
		if (event == NULL) {
			continue;
		}

		double value = IOHIDEventGetFloatValue(event, IOHIDEventFieldBase(kIOHIDEventTypeTemperature));
		CFRelease(event);
		if (!is_reasonable_temperature(value)) {
			continue;
		}

		fallback_sum += value;
		if (fallback_count == 0 || value > fallback_max) {
			fallback_max = value;
		}
		fallback_count++;

		CFStringRef product = IOHIDServiceClientCopyProperty(service, CFSTR("Product"));
		char name[128] = {0};
		if (product != NULL) {
			(void)cfstring_to_cstring(product, name, sizeof(name));
			CFRelease(product);
		}

		if (is_soc_temperature_sensor(name)) {
			primary_sum += value;
			if (primary_count == 0 || value > primary_max) {
				primary_max = value;
			}
			primary_count++;
		}
	}

	CFRelease(services);
	CFRelease(system);

	if (primary_count > 0) {
		sample->avg_temp_c = primary_sum / (double)primary_count;
		sample->max_temp_c = primary_max;
		sample->temp_sensor_count = primary_count;
		return;
	}
	if (fallback_count > 0) {
		sample->avg_temp_c = fallback_sum / (double)fallback_count;
		sample->max_temp_c = fallback_max;
		sample->temp_sensor_count = fallback_count;
	}
}

static uint32_t str_to_uint32(const char *key) {
	return ((uint32_t)key[0] << 24) | ((uint32_t)key[1] << 16) | ((uint32_t)key[2] << 8) | (uint32_t)key[3];
}

static void uint32_to_str(uint32_t value, char out[5]) {
	out[0] = (char)((value >> 24) & 0xff);
	out[1] = (char)((value >> 16) & 0xff);
	out[2] = (char)((value >> 8) & 0xff);
	out[3] = (char)(value & 0xff);
	out[4] = '\0';
}

static kern_return_t smc_call(uint8_t index, SMCKeyData *input, SMCKeyData *output, io_connect_t conn) {
	size_t input_size = sizeof(SMCKeyData);
	size_t output_size = sizeof(SMCKeyData);
	return IOConnectCallStructMethod(conn, (uint32_t)index, input, input_size, output, &output_size);
}

static kern_return_t smc_open(io_connect_t *conn) {
	mach_port_t main_port;
	io_iterator_t iterator = 0;
	io_object_t device = 0;

	IOMainPort(MACH_PORT_NULL, &main_port);

	kern_return_t result = IOServiceGetMatchingServices(main_port, IOServiceMatching("AppleSMC"), &iterator);
	if (result != kIOReturnSuccess) {
		return result;
	}

	device = IOIteratorNext(iterator);
	IOObjectRelease(iterator);
	if (device == 0) {
		return kIOReturnNotFound;
	}

	result = IOServiceOpen(device, mach_task_self_, 0, conn);
	IOObjectRelease(device);
	return result;
}

static kern_return_t smc_read_key(const char *key, SMCVal *value, io_connect_t conn) {
	SMCKeyData input = {0};
	SMCKeyData output = {0};

	snprintf(value->key, sizeof(value->key), "%s", key);
	input.key = str_to_uint32(key);
	input.data8 = SMC_CMD_READ_KEYINFO;

	kern_return_t result = smc_call(KERNEL_INDEX_SMC, &input, &output, conn);
	if (result != kIOReturnSuccess) {
		return result;
	}

	value->dataSize = output.keyInfo.dataSize;
	value->dataType = output.keyInfo.dataType;

	input.keyInfo.dataSize = output.keyInfo.dataSize;
	input.data8 = SMC_CMD_READ_BYTES;

	result = smc_call(KERNEL_INDEX_SMC, &input, &output, conn);
	if (result != kIOReturnSuccess) {
		return result;
	}

	memcpy(value->bytes, output.bytes, sizeof(output.bytes));
	return kIOReturnSuccess;
}

static bool smc_decode_value(const SMCVal *value, double *out) {
	char data_type[5] = {0};
	uint32_to_str(value->dataType, data_type);

	if (strcmp(data_type, "ui8 ") == 0) {
		*out = (double)value->bytes[0];
		return true;
	}
	if (strcmp(data_type, "ui16") == 0) {
		*out = (double)(((uint16_t)value->bytes[0] << 8) | value->bytes[1]);
		return true;
	}
	if (strcmp(data_type, "ui32") == 0) {
		*out = (double)(((uint32_t)value->bytes[0] << 24) | ((uint32_t)value->bytes[1] << 16) | ((uint32_t)value->bytes[2] << 8) | value->bytes[3]);
		return true;
	}
	if (strcmp(data_type, "sp78") == 0) {
		int16_t raw = (int16_t)(((uint16_t)value->bytes[0] << 8) | value->bytes[1]);
		*out = (double)raw / 256.0;
		return true;
	}
	if (strcmp(data_type, "flt ") == 0) {
		float raw = 0;
		memcpy(&raw, value->bytes, sizeof(float));
		*out = (double)raw;
		return isfinite(*out);
	}
	if (strcmp(data_type, "fpe2") == 0) {
		*out = (double)(((int)value->bytes[0] << 6) + ((int)value->bytes[1] >> 2));
		return true;
	}
	return false;
}

static bool smc_read_double(io_connect_t conn, const char *key, double *out) {
	SMCVal value = {0};
	kern_return_t result = smc_read_key(key, &value, conn);
	if (result != kIOReturnSuccess) {
		return false;
	}
	return smc_decode_value(&value, out);
}

static void collect_fans(WattsHWMonSample *sample) {
	io_connect_t conn = 0;
	kern_return_t result = smc_open(&conn);
	if (result != kIOReturnSuccess) {
		char message[256];
		snprintf(message, sizeof(message), "open AppleSMC: %s", mach_error_string(result));
		set_error(sample, message);
		return;
	}

	double fan_count = 0.0;
	if (!smc_read_double(conn, "FNum", &fan_count) || fan_count <= 0) {
		set_error(sample, "read FNum from AppleSMC");
		IOServiceClose(conn);
		return;
	}

	sample->fan_count = (int)fan_count;
	double value = 0.0;
	if (sample->fan_count >= 1 && smc_read_double(conn, "F0Ac", &value) && value > 0) {
		sample->fan0_rpm = value;
	}
	if (sample->fan_count >= 2 && smc_read_double(conn, "F1Ac", &value) && value > 0) {
		sample->fan1_rpm = value;
	}

	IOServiceClose(conn);
}

WattsHWMonSample watts_hwmon_sample(void) {
	WattsHWMonSample sample;
	memset(&sample, 0, sizeof(sample));

	collect_temperatures(&sample);
	collect_fans(&sample);

	return sample;
}
