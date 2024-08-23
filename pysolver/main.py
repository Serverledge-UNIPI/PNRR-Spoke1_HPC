from flask import Flask, request, jsonify
from cp_sat_solver_rate import start_solver
from earliest_deadline_first_solver import start_edf
import argparse

app = Flask(__name__)

@app.route('/solve_with_cp_sat', methods=['POST'])
def solve():
    try:
        data = request.json
        number_of_nodes = data['number_of_nodes']
        number_of_functions = data['number_of_functions']
        node_memory = data['node_memory']
        node_capacity = data['node_capacity']
        maximum_capacity = data['maximum_capacity']
        node_ipc = data['node_ipc']
        node_power_consumption = data['node_power_consumption']
        function_memory = data['function_memory']
        function_workload = data['function_workload']
        function_deadline = data['function_deadline']
        function_invocations = data['function_invocations']
        
        results = start_solver(number_of_nodes, number_of_functions, node_memory, node_capacity, maximum_capacity, node_ipc, node_power_consumption,
            function_memory, function_workload, function_deadline, function_invocations)
        
        return jsonify(results)
    except Exception as e:
        return jsonify({'error': str(e)}), 400
    
@app.route('/solve_with_edf', methods=['POST'])
def solve_edf():
    try:
        data = request.json
        number_of_nodes = data['number_of_nodes']
        number_of_functions = data['number_of_functions']
        node_memory = data['node_memory']
        node_capacity = data['node_capacity']
        maximum_capacity = data['maximum_capacity']
        node_ipc = data['node_ipc']
        node_power_consumption = data['node_power_consumption']
        function_memory = data['function_memory']
        function_workload = data['function_workload']
        function_deadline = data['function_deadline']
        function_invocations = data['function_invocations']
        
        nodes = [
            {
                'id': i,
                'total_memory': node_memory[i],
                'total_capacity': node_capacity[i],
                'maximum_capacity': maximum_capacity[i],
                'power_consumption': node_power_consumption[i],
                'ipc': node_ipc[i]
            } for i in range(number_of_nodes)
        ]

        functions = [
            {
                'id': i,
                'memory': function_memory[i],
                'workload': function_workload[i],
                'deadline': function_deadline[i],
                'invocations': function_invocations[i]
            } for i in range(number_of_functions)
        ]
        
        results = start_edf(functions, nodes)

        return jsonify(results)
    except Exception as e:
        return jsonify({'error': str(e)}), 400

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Run the Flask application')
    parser.add_argument('--port', type=int, default=5000, help='Port to run the application on')
    args = parser.parse_args()
    
    app.run(debug=True, host='0.0.0.0', port=args.port)
