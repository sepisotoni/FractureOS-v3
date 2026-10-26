"""Protocol tests for fragmentation, escaping and connection failure handling."""
import socket
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "tools"))
from tomtom import TelnetStream, TomTom


class TelnetTests(unittest.TestCase):
    def test_every_fragment_boundary(self):
        wire = b"\xff\xfd\x1f\xff\xfb\x01\xff\xfb\x03\r\nlogin: "
        for split in range(len(wire)+1):
            stream = TelnetStream()
            a, ar = stream.feed(wire[:split])
            b, br = stream.feed(wire[split:])
            self.assertEqual(a+b, b"\r\nlogin: ")
            self.assertEqual(ar+br, b"\xff\xfc\x1f\xff\xfd\x01\xff\xfd\x03")

    def test_subnegotiation_and_literal_iac_byte_by_byte(self):
        stream = TelnetStream()
        wire = b"A\xff\xfa\x1f\x00\xff\xff\x00\xff\xf0B\xff\xffC"
        plain = b"".join(stream.feed(bytes([x]))[0] for x in wire)
        self.assertEqual(plain, b"AB\xffC")

    def test_refuse_unknown_options_without_refusal_loop(self):
        stream = TelnetStream()
        plain, reply = stream.feed(b"\xff\xfb\x42\xff\xfd\x42\xff\xfc\x42\xff\xfe\x42")
        self.assertEqual(plain, b"")
        self.assertEqual(reply, b"\xff\xfe\x42\xff\xfc\x42")

    def make_connection(self):
        local, remote = socket.socketpair()
        self.addCleanup(local.close)
        self.addCleanup(remote.close)
        device = TomTom.__new__(TomTom)
        device.sock, device.parser, device.buffer = local, TelnetStream(), bytearray()
        return device, remote

    def test_eof_is_failure_not_busy_loop(self):
        device, remote = self.make_connection()
        remote.close()
        with self.assertRaises(ConnectionError):
            device.until(rb"missing", 1)

    def test_output_limit(self):
        device, remote = self.make_connection()
        remote.sendall(b"x" * 20)
        with self.assertRaisesRegex(RuntimeError, "exceeded limit"):
            device.until(rb"missing", 1, limit=10)

    def test_nvt_cr_normalization(self):
        device, remote = self.make_connection()
        remote.sendall(b"first\r\r\n__DONE__=7\r\n")
        data, match = device.until(rb"\n__DONE__=(\d+)\n", 1)
        self.assertEqual(data[:match.start()], b"first")
        self.assertEqual(match.group(1), b"7")


if __name__ == "__main__":
    unittest.main()
