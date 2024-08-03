from flask import Flask, request, jsonify
from solver import start_solver
import argparse

app = Flask(__name__)

@app.route('/solve', methods=['POST'])
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

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Run the Flask application')
    parser.add_argument('--port', type=int, default=5000, help='Port to run the application on')
    args = parser.parse_args()
    
    app.run(debug=True, host='0.0.0.0', port=args.port)
