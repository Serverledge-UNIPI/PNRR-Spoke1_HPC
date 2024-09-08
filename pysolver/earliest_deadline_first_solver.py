import yaml

CONFIG_PATH = 'configuration.yaml'

# Colors for the prints (if needed)
RED = '\033[91m'
GREEN = '\033[92m'
RESET = '\033[0m'

def log(message, logging):
    if logging:
        print(f'{message}')

def start_edf(functions, nodes):
    with open(CONFIG_PATH, 'r') as file:
        data = yaml.safe_load(file)

    solver_constants = data.get('constants', {})['solver']
    logging = solver_constants['console_logging']

    state = "FEASIBLE"

    node_instances = {i: [0] * len(functions) for i in range(len(nodes))}
    function_capacities = {i: [0] * len(nodes) for i in range(len(functions))}

    # Order functions by their deadline
    functions.sort(key=lambda f: f['deadline'])

    # Order nodes by their capacity and memory    
    nodes.sort(key=lambda n: (n['total_capacity'], n['total_memory']), reverse=True)
    
    for function in functions:
        placed_invocations = 0
        for _ in range(0, function['peak_invocations']):
            placed = False
            for node in nodes:
                function_required_capacity = function['workload'] / ((function['deadline'] / 1000) * (node['ipc']/10))
                if (node['total_memory'] >= function['memory'] and node['total_capacity'] >= function_required_capacity):

                    if 'hosted_functions' not in node:
                        node['hosted_functions'] = {}
                        node['is_active'] = True

                    if function['id'] in node['hosted_functions']:
                        node['hosted_functions'][function['id']]['peak_invocations'] += 1
                    else:
                        node['hosted_functions'][function['id']] = {
                            'peak_invocations': 1,
                            'capacity_assigned': function_required_capacity
                        }
                    
                    node_instances[node['id']][function['id']] = node['hosted_functions'][function['id']]['peak_invocations']
                    function_capacities[function['id']][node['id']] = function_required_capacity
                    
                    node['total_memory'] -= function['memory']
                    node['total_capacity'] -= function_required_capacity
                    placed = True
                    placed_invocations += 1
                    break
            if not placed:
                state = "INFEASIBLE"
                break

    for node in nodes:
        status = f"{GREEN}Active{RESET}" if node.get('is_active', False) else f"{RED}Inactive{RESET}"
        log(f'Node {node["id"]} - {status}', logging)
        
        if 'hosted_functions' in node:
            for function_id, details in node['hosted_functions'].items():
                function = next(f for f in functions if f['id'] == function_id)
                log(f'   Function {function_id}: Instances={details["peak_invocations"]}, Capacity={details["capacity_assigned"]:.2f} Mhz, Deadline: {(function["deadline"]/1000):.3f} s', logging)
        
        log(f'   Memory: {node["total_memory"]:.2f} (Mb), Capacity: {node["total_capacity"] / (10 ** 3)} (Ghz), IPC: {node["ipc"]}, Power consumption: {node["power_consumption"]} (Watt)', logging)
        log(f'   Memory available: {node["total_memory"]} (Mb), Capacity available: {node["total_capacity"] / (10 ** 3):.2f} (Ghz)\n', logging)

    if state == "INFEASIBLE":
        node_instances = {i: [0] * len(functions) for i in range(len(nodes))}
        function_capacities = {i: [0] * len(nodes) for i in range(len(functions))}
        
        for node in nodes:
            node['is_active'] = True

    results = {
        'solver_status_name': state,
        'solver_walltime': None,
        'objective_value': sum(int(node.get('is_active', False)) * node['power_consumption'] for node in nodes),
        'active_nodes_indexes': [1 if node.get('is_active', False) else 0 for node in nodes],
        'node_instances': node_instances,
        'function_capacities': function_capacities
    }

    return results
