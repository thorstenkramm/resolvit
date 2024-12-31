#!/usr/bin/env python3

import argparse
import concurrent.futures
import dns.resolver
import random
import string
import time
from dataclasses import dataclass
from typing import Optional


@dataclass
class TestResult:
    success: int = 0
    failures: int = 0
    duration: float = 0.0


def generate_random_string(length: int = 12) -> str:
    return ''.join(random.choices(string.ascii_lowercase, k=length))


def perform_dns_query(resolver: dns.resolver.Resolver, query: str,
                      expect_content: Optional[str]) -> bool:
    try:
        answers = resolver.resolve(query)
        if expect_content:
            return str(answers[0]) == expect_content
        return True
    except Exception:
        return False


def run_stress_test(server: str, port: int, query: str,
                    expect_content: Optional[str], num_requests: int,
                    concurrency: int) -> TestResult:
    resolver = dns.resolver.Resolver()
    resolver.nameservers = [server]
    resolver.port = port

    result = TestResult()
    start_time = time.time()

    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as executor:
        futures = []
        for _ in range(num_requests):
            test_query = query.replace('%RAND%', generate_random_string())
            futures.append(
                executor.submit(perform_dns_query, resolver, test_query, expect_content)
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
    parser.add_argument('--server', required=True, help='DNS server IP or hostname')
    parser.add_argument('--port', type=int, default=53, help='DNS server port')
    parser.add_argument('--query', required=True, help='Query to test (use %%RAND%% for random strings)')
    parser.add_argument('--expect-content', help='Expected record content')
    parser.add_argument('--num-requests', type=int, default=100, help='Total number of requests')
    parser.add_argument('--concurrency', type=int, default=10, help='Number of concurrent workers')

    args = parser.parse_args()

    print(f"Starting DNS stress test against {args.server}:{args.port}")
    print(f"Query: {args.query}")
    print(f"Requests: {args.num_requests}, Concurrency: {args.concurrency}")

    result = run_stress_test(
        args.server, args.port, args.query,
        args.expect_content, args.num_requests, args.concurrency
    )

    print("\nResults:")
    print(f"Duration: {result.duration:.2f} seconds")
    print(f"Successful queries: {result.success}")
    print(f"Failed queries: {result.failures}")
    print(f"Queries per second: {args.num_requests / result.duration:.2f}")

    exit(1 if result.failures > 0 else 0)


if __name__ == '__main__':
    main()
