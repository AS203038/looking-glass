import { beforeEach, describe, expect, it } from 'vitest';
import { fireEvent, render } from '@testing-library/svelte';
import { fromJson } from '@bufbuild/protobuf';
import * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';
import { _reset as resetRouters, routers, selectMany } from '$lib/stores/routers';
import { _resetQuery, command, parameter } from '$lib/stores/query';
import CommandDock from './CommandDock.svelte';

describe('CommandDock', () => {
	beforeEach(() => {
		resetRouters();
		_resetQuery();
		routers.set([
			fromJson(Pb.RouterSchema, {
				id: '1',
				name: 'ams-01',
				location: 'ams',
				health: { healthy: true }
			})
		]);
		selectMany([1n]);
	});

	it('disables parameter input when bgp_summary is selected and enables Execute', async () => {
		const { getByLabelText, getByRole } = render(CommandDock);
		const select = getByLabelText('Action') as HTMLSelectElement;
		await fireEvent.change(select, { target: { value: 'bgp_summary' } });

		const paramInput = getByLabelText('Parameter') as HTMLInputElement;
		expect(paramInput.disabled).toBe(true);
		const execBtn = getByRole('button', { name: 'Execute' }) as HTMLButtonElement;
		expect(execBtn.disabled).toBe(false);
	});

	it('validates BGP standard and large community integer bounds', async () => {
		const { getByLabelText, getByRole, findByText } = render(CommandDock);
		await fireEvent.change(getByLabelText('Action'), { target: { value: 'bgp_community' } });

		const paramInput = getByLabelText('Parameter') as HTMLInputElement;
		await fireEvent.input(paramInput, { target: { value: '65000:999999' } });

		expect(await findByText(/between 0 and 65535/i)).toBeTruthy();
		expect((getByRole('button', { name: 'Execute' }) as HTMLButtonElement).disabled).toBe(true);

		await fireEvent.input(paramInput, { target: { value: '203038:100:200' } });
		expect(await findByText(/Large Community/i)).toBeTruthy();
		expect((getByRole('button', { name: 'Execute' }) as HTMLButtonElement).disabled).toBe(false);
	});

	it('renders structured Peer IP, Peer Name, and RIB controls for bgp_peer_routes', async () => {
		command.set('bgp_peer_routes');
		parameter.set('');
		const { getByLabelText, getByRole } = render(CommandDock);

		const nameInput = getByLabelText('Peer Name') as HTMLInputElement;
		const ribSelect = getByLabelText('RIB') as HTMLSelectElement;
		await fireEvent.input(nameInput, { target: { value: 'peer-a' } });
		await fireEvent.change(ribSelect, { target: { value: 'accepted' } });

		expect((getByRole('button', { name: 'Execute' }) as HTMLButtonElement).disabled).toBe(false);
	});
});
