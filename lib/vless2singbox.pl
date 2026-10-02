#!/usr/bin/perl
# Ссылка vless:// -> конфиг sing-box: tun, весь трафик через VLESS.
# Usage: vless2singbox.pl 'vless://...' [адрес-в-обход-туннеля ...]
use strict;
use warnings;
use JSON::PP;

my ($link, @exclude) = @ARGV;
die "Usage: $0 'vless://...' [адрес-в-обход ...]\n" unless $link;

sub unesc {
	my $s = shift // '';
	$s =~ tr/+/ /;
	$s =~ s/%([0-9A-Fa-f]{2})/chr hex $1/ge;
	return $s;
}

$link =~ m{^vless://([^@]+)@(\[[^\]]+\]|[^:/?#]+):(\d+)/?(?:\?([^#]*))?(?:#.*)?$}
	or die "Не похоже на ссылку vless://uuid\@host:port?...\n";
my ($uuid, $host, $port, $query) = (unesc($1), $2, $3, $4 // '');
$host =~ s/^\[|\]$//g;
my %p = map { my ($k, $v) = split /=/, $_, 2; ($k => unesc($v)) } grep { length } split /&/, $query;

my $type = $p{type} || 'tcp';
die "Транспорт '$type' пока не поддержан, только tcp\n" unless $type eq 'tcp';

my %out = (
	type        => 'vless',
	tag         => 'proxy',
	server      => $host,
	server_port => $port + 0,
	uuid        => $uuid,
);
$out{flow} = $p{flow} if $p{flow};

my $security = $p{security} || 'none';
if ($security eq 'reality' || $security eq 'tls') {
	my %tls = (enabled => JSON::PP::true);
	$tls{server_name} = $p{sni} if $p{sni};
	$tls{utls} = { enabled => JSON::PP::true, fingerprint => $p{fp} } if $p{fp};
	if ($security eq 'reality') {
		die "В ссылке нет pbk (публичный ключ reality)\n" unless $p{pbk};
		$tls{reality} = {
			enabled    => JSON::PP::true,
			public_key => $p{pbk},
			($p{sid} ? (short_id => $p{sid}) : ()),
		};
	}
	$out{tls} = \%tls;
} elsif ($security ne 'none') {
	die "security '$security' не поддержан\n";
}

my %tun = (
	type         => 'tun',
	tag          => 'tun-in',
	# Вне корп-сетей 10/8 и 172.16/12, чтобы не пересекаться с cisco.
	address      => ['198.18.0.1/30'],
	auto_route   => JSON::PP::true,
	strict_route => JSON::PP::false,
	stack        => 'system',
);
# Linux: имя закрепляем, иначе после рестарта sing-box берёт первый свободный
# tunN, а на имена завязан фаервол хоста. tun1 — cisco (cvpn, CISCO_TUN).
$tun{interface_name} = 'tun0' if $^O eq 'linux';
$tun{route_exclude_address} = [map { m{/} ? $_ : "$_/32" } @exclude] if @exclude;

# DNS. На Linux sing-box регистрирует свой tun в systemd-resolved как DNS для
# всех доменов, поэтому запросы надо перехватить и ответить самому: DoH через
# VLESS — заодно без подмены ответов провайдером. bootstrap нужен только
# чтобы резолвить адрес сервера VLESS, если в ссылке домен, а не IP.
my $config = {
	log => { level => 'warn' },
	dns => {
		servers => [
			{ type => 'https', tag => 'remote', server => '1.1.1.1', detour => 'proxy' },
			{ type => 'udp', tag => 'bootstrap', server => '1.1.1.1' },
		],
		final => 'remote',
	},
	inbounds  => [\%tun],
	outbounds => [\%out, { type => 'direct', tag => 'direct' }],
	route     => {
		auto_detect_interface   => JSON::PP::true,
		default_domain_resolver => 'bootstrap',
		rules                   => [
			{ action   => 'sniff' },
			{ protocol => 'dns', action => 'hijack-dns' },
			{ ip_is_private => JSON::PP::true, outbound => 'direct' },
		],
		final => 'proxy',
	},
};
print JSON::PP->new->pretty->canonical->encode($config);
