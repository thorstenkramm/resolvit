#!/usr/bin/env python3

import argparse
import concurrent.futures
import dns.exception
import dns.flags
import dns.message
import dns.query
import dns.rcode
import dns.rdatatype
import random
import string
import time
from dataclasses import dataclass
from typing import Optional

# Same limits the dnspython stub resolver applies by default.
QUERY_TIMEOUT = 2.0
QUERY_ATTEMPTS = 3


@dataclass
class TestResult:
    success: int = 0
    failures: int = 0
    duration: float = 0.0


@dataclass
class Expectation:
    rcode: int
    content: Optional[str] = None
    empty: bool = False


def generate_random_string(length: int = 12) -> str:
    return ''.join(random.choices(string.ascii_lowercase, k=length))


def exchange(server: str, port: int, query: str, qtype: int) -> Optional[dns.message.Message]:
    # The raw reply is needed: the stub resolver turns NXDOMAIN, SERVFAIL and empty answers into exceptions.
    request = dns.message.make_query(query, qtype)
    for _ in range(QUERY_ATTEMPTS):
        try:
            response = dns.query.udp(request, server, port=port, timeout=QUERY_TIMEOUT)
            if response.flags & dns.flags.TC:
                response = dns.query.tcp(request, server, port=port, timeout=QUERY_TIMEOUT)
            return response
        except (dns.exception.DNSException, OSError):
            continue
    return None


def perform_dns_query(server: str, port: int, query: str, qtype: int,
                      expect: Expectation) -> bool:
    response = exchange(server, port, query, qtype)
    if response is None or response.rcode() != expect.rcode:
        return False
    if expect.empty:
        return len(response.answer) == 0
    if expect.content:
        for rrset in response.answer:
            if rrset.rdtype == qtype:
                return rrset[0].to_text() == expect.content
        return False
    return True


def run_stress_test(server: str, port: int, query: str, qtype: int,
                    expect: Expectation, num_requests: int,
                    concurrency: int) -> TestResult:
    result = TestResult()
    start_time = time.time()

    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as executor:
        futures = []
        for _ in range(num_requests):
            test_query = query.replace('%RAND%', generate_random_string())
            futures.append(
                executor.submit(perform_dns_query, server, port, test_query, qtype, expect)
            )

        for future in concurrent.futures.as_completed(futures):
            if future.result():
                result.success += 1
            else:
                result.failures += 1

    result.duration = time.time() - start_time
    return result


def main():
    parser = argparse.ArgumentParser(description='DNS server stress testing tool')
    parser.add_argument('--server', required=True, help='DNS server IP address')
    parser.add_argument('--port', type=int, default=53, help='DNS server port')
    parser.add_argument('--query', required=True, help='Query to test (use %%RAND%% for random strings)')
    parser.add_argument('--qtype', default='A', help='Record type to ask for')
    parser.add_argument('--expect-rcode', default='NOERROR', help='Expected response code, e.g. NXDOMAIN or SERVFAIL')
    parser.add_argument('--expect-content', help='Expected content of the first record of the queried type')
    parser.add_argument('--expect-empty', action='store_true', help='Expect an empty answer section')
    parser.add_argument('--num-requests', type=int, default=100, help='Total number of requests')
    parser.add_argument('--concurrency', type=int, default=10, help='Number of concurrent workers')

    args = parser.parse_args()
    if args.expect_empty and args.expect_content:
        parser.error('--expect-empty and --expect-content exclude each other')

    qtype = dns.rdatatype.from_text(args.qtype)
    expect = Expectation(
        rcode=dns.rcode.from_text(args.expect_rcode),
        content=args.expect_content,
        empty=args.expect_empty,
    )

    print(f"Starting DNS stress test against {args.server}:{args.port}")
    print(f"Query: {args.query} {args.qtype}, expecting {args.expect_rcode}")
    print(f"Requests: {args.num_requests}, Concurrency: {args.concurrency}")

    result = run_stress_test(
        args.server, args.port, args.query, qtype,
        expect, args.num_requests, args.concurrency
    )

    print("\nResults:")
    print(f"Duration: {result.duration:.2f} seconds")
    print(f"Successful queries: {result.success}")
    print(f"Failed queries: {result.failures}")
    print(f"Queries per second: {args.num_requests / result.duration:.2f}")

    exit(1 if result.failures > 0 else 0)


if __name__ == '__main__':
    main()
