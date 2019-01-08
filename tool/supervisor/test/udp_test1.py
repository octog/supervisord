import socket

sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)

sock.bind(('127.0.0.1', 3001))

print ('start server on [%s]:%s' % ('127.0.0.1', 3001))

while True:
    data, addr = sock.recvfrom(1024)
    print ('Received from %s:%s' % (addr, data))
    sock.sendto(b'Hello, %s!' % data, addr)
