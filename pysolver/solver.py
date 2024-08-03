from ortools.sat.python import cp_model
import yaml

CONFIG_PATH = 'configuration.yaml'

# Colors for the prints
RED = '\033[91m'
GREEN = '\033[92m'
RESET = '\033[0m'

def log(message, logging):
    if logging:
        print(f'{message}')

def start_solver(number_of_nodes: int, number_of_functions: int, node_memory: list, node_capacity: list, maximum_capacity: list, node_ipc: list, node_power_consumption: list, 
                 function_memory: list, function_workload: list,  function_deadline: list, function_invocations: list) -> dict:
    # Retrieve CP-SAT parameters from the configuration file
    with open(CONFIG_PATH, 'r') as file:
        data = yaml.safe_load(file)

    solver_constants = data.get('constants', {})['solver']
    logging = solver_constants['console_logging']
    
    total_system_memory_available = sum(node_memory[i] for i in range(number_of_nodes))
    total_system_capacity_available = sum(node_capacity[i] for i in range(number_of_nodes))
    
    # Create the model
    model = cp_model.CpModel()

    # Create decision variables
    max_n = 10000
    y = {i: model.new_bool_var(f'y_{i}') for i in range(number_of_nodes)}
    c = {(i, j): model.new_int_var(0, max_n * maximum_capacity[i], f'c_{i}_{j}') for i in range(number_of_nodes) for j in range(number_of_functions)}
    n = {(i, j): model.new_int_var(0, max_n, f'n_{i}_{j}') for i in range(number_of_nodes) for j in range(number_of_functions)}

    # Define constraints
    for i in range(number_of_nodes):
        for j in range(number_of_functions):
            # Deadline constraint
            model.add(10000 * n[i, j] * function_workload[j] <= function_deadline[j] * node_ipc[i] * c[i, j])

    for i in range(number_of_nodes):
            # Memory constraint
            model.add(sum(n[i, j] * function_memory[j] for j in range(number_of_functions)) <= node_memory[i] * y[i])
            
            # Capacity constraint
            model.add(sum(c[i, j] for j in range(number_of_functions)) <= node_capacity[i] * y[i])

    for j in range(number_of_functions):
        # Number of requests
        model.add(sum(n[i, j] for i in range(number_of_nodes)) == function_invocations[j])

    # The goal is to minimize the number of active nodes
    model.minimize(sum(y[i] * node_power_consumption[i] for i in range(number_of_nodes)))

    # Create the solver and solve the model
    solver = cp_model.CpSolver()
    solver.parameters.num_search_workers = solver_constants['search_workers']
    solver.parameters.log_search_progress = solver_constants['log_search_progress']

    if solver_constants['max_simulation_time']:
        solver.parameters.max_time_in_seconds = solver_constants['max_simulation_time'] 

    status = solver.solve(model)

    system_memory_utilization = 0
    system_capacity_utilization = 0
    nodes_capacity_utilization = [] # Utilization per node
    nodes_instances = {i: [0] * number_of_functions for i in range(number_of_nodes)}
    functions_capacity = [0] * number_of_functions
    
    if status == cp_model.OPTIMAL or status == cp_model.FEASIBLE:    
        for i in range(number_of_nodes):
            total_memory_assigned = 0
            total_capacity_assigned = 0

            log(f'Node {i} - {f"{GREEN}Active{RESET}" if solver.value(y[i]) else f"{RED}Inactive{RESET}"}', logging)
            for j in range(number_of_functions):                
                if solver.value(n[i, j]) and solver.value(c[i, j]):
                    single_instance_capacity = solver.value(c[i, j]) / solver.value(n[i, j])
                    execution_time = function_workload[j] / (single_instance_capacity * (node_ipc[i]/10))
                
                    total_capacity_assigned += solver.value(c[i, j])  
                    total_memory_assigned += solver.value(n[i, j]) * function_memory[j]

                    nodes_instances[i][j] = solver.value(n[i, j])
                    functions_capacity[j] = single_instance_capacity
                
                    log(f'   Function {j + 1}: Instances={solver.value(n[i, j])}, Capacity={single_instance_capacity:.2f} Mhz, Execution time: {execution_time:.3f} s, Deadline: {(function_deadline[j]/1000):.3f} s', logging)

            log(f'   Memory: {node_memory[i]} (Mb), Capacity: {node_capacity[i]/(10 ** 3)} (Ghz), IPC: {node_ipc[i]/10}, Power consumption: {node_power_consumption[i]} (Watt)', logging)
            log(f'   Memory available: {node_memory[i] - total_memory_assigned} (Mb), Capacity available: {(node_capacity[i] - total_capacity_assigned)/(10 ** 3)} (Ghz)\n', logging)
    
            nodes_capacity_utilization.append((total_capacity_assigned / node_capacity[i]) * 100)

        # Obtain objective function results
        active_nodes = [solver.value(y[i]) for i in range(number_of_nodes)]
        
        # Obtain system utilization
        system_capacity_assigned = sum(solver.value(c[i, j]) for j in range(number_of_functions) for i in range (number_of_nodes))
        system_capacity_available = total_system_capacity_available - system_capacity_assigned
        system_capacity_utilization = (system_capacity_assigned/total_system_capacity_available) * 100

        system_memory_assigned = sum(function_memory[j] * function_invocations[j] for j in range(number_of_functions))
        system_memory_available = total_system_memory_available - system_memory_assigned
        system_memory_utilization = (system_memory_assigned/total_system_memory_available) * 100

        objective_value = int(solver.ObjectiveValue())
    else:     
        # In order to understand how many capacity is still needed, it is estimated supposing that 
        # all the functions are executed on a node having an IPC equal to the average of the IPCs of the nodes in the scenario
        capacity_needed_estimation = 0
        average_ipc = (sum(node_ipc) / len(node_ipc)) / 10
        memory_needed = 0
        
        for i in range(number_of_functions):
            capacity_estimation = function_workload[i] / ((function_deadline[i] / 1000) * average_ipc)
            capacity_needed_estimation += capacity_estimation * function_invocations[i]
            memory_needed += function_memory[i] * function_invocations[i]
            
        system_memory_available = total_system_memory_available - memory_needed
        system_capacity_available = total_system_capacity_available - capacity_needed_estimation
        
        # If no solution available, all nodes must be activated and will be at full utilization
        system_memory_utilization = 100
        system_capacity_utilization = 100
        nodes_capacity_utilization = [100] * number_of_nodes
        active_nodes = [1] * number_of_nodes
        objective_value = sum(node_power_consumption)
    
    results = {
        'solver_status_name': solver.StatusName(status),
        'solver_walltime': solver.WallTime(),
        'objective_value': objective_value,
        #'system_memory_utilization': system_memory_utilization,
        #'system_capacity_utilization': system_capacity_utilization,
        #'nodes_capacity_utilization': nodes_capacity_utilization,
        #'system_memory_available': system_memory_available / 1024,
        #'system_capacity_available': system_capacity_available / (10 ** 9),
        #'active_nodes': sum(active_nodes),
        'active_nodes_indexes': active_nodes,
        'nodes_instances': nodes_instances,
        'functions_capacity': functions_capacity
    }

    return results