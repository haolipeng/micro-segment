# Micro-Segment DP Makefile
# Simplified version for micro-segment project

CC = gcc
CFLAGS = -Wall -O2 -g -fPIC
CFLAGS += -D_GNU_SOURCE
CFLAGS += -I. -I./dpi -I./utils

# Libraries
LDFLAGS = -ljansson -lurcu -lurcu-cds -lnetfilter_queue -lpthread -lrt -lm -lpcre2-8

# Source files
SRCS = main.c debug.c pkt.c ring.c
SRCS += ctrl/ctrl.c
SRCS += nfq/nfq.c
SRCS += dpi/dpi_policy.c dpi/dpi_session.c dpi/dpi_packet.c
SRCS += dpi/dpi_entry.c dpi/dpi_frag.c dpi/dpi_log.c dpi/dpi_meter.c dpi/dpi_msg.c dpi/dpi_parser.c dpi/dpi_debug.c
SRCS += dpi/parsers/dpi_cassandra.c dpi/parsers/dpi_couchbase.c dpi/parsers/dpi_dhcp.c dpi/parsers/dpi_dns.c
SRCS += dpi/parsers/dpi_echo.c dpi/parsers/dpi_grpc.c dpi/parsers/dpi_http.c dpi/parsers/dpi_kafka.c
SRCS += dpi/parsers/dpi_mongodb.c dpi/parsers/dpi_mysql.c dpi/parsers/dpi_ntp.c dpi/parsers/dpi_postgresql.c
SRCS += dpi/parsers/dpi_redis.c dpi/parsers/dpi_spark.c dpi/parsers/dpi_sqlinjection.c dpi/parsers/dpi_ssh.c
SRCS += dpi/parsers/dpi_ssl.c dpi/parsers/dpi_tds.c dpi/parsers/dpi_tftp.c dpi/parsers/dpi_tns.c
SRCS += dpi/parsers/dpi_zookeeper.c
SRCS += utils/rcu_map.c utils/helper.c utils/timer_queue.c utils/bitmap.c utils/timer_wheel.c utils/tree.c utils/asm.c utils/asn1.c

# Object files
OBJS = $(SRCS:.c=.o)

# Target
TARGET = ../../bin/dp

all: $(TARGET)

$(TARGET): $(OBJS)
	@mkdir -p $(dir $(TARGET))
	$(CC) $(CFLAGS) -o $@ $^ $(LDFLAGS)
	@echo "DP built successfully: $(TARGET)"

%.o: %.c
	@echo "Compiling $<..."
	$(CC) $(CFLAGS) -c $< -o $@

clean:
	@echo "Cleaning DP..."
	rm -f $(OBJS) $(TARGET)
	@echo "Clean completed"

.PHONY: all clean
