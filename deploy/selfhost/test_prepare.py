import unittest

from prepare import upstream_tls_name


class UpstreamTlsNameTests(unittest.TestCase):
    def test_ip_uses_localhost_dns_san_for_nginx(self):
        self.assertEqual(upstream_tls_name('8.156.66.175'), 'localhost')

    def test_dns_player_host_keeps_its_name(self):
        self.assertEqual(upstream_tls_name('player.example.com'), 'player.example.com')


if __name__ == '__main__':
    unittest.main()
