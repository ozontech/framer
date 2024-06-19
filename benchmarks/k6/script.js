import { check, sleep } from 'k6';
import { Client, StatusOK } from 'k6/net/grpc';

export const options = {
  vus: 10,
  duration: '20s',
};

const client = new Client();
client.load([], '../dumb-server/api/api.proto');

export default function() {
	if (__ITER == 0) {
		client.connect('127.0.0.1:9090', { plaintext: true });
	}

	const data = { field: 'ping' };
	const params = {
		metadata: {
			'x-my-header-key1': 'my-header-val1',
			'x-my-header-key2': 'my-header-val2',
		},
	};
	const response = client.invoke('test.api.TestApi/Test', data);
	check(response, { 'status is OK': (r) => r && r.status === StatusOK })
}
