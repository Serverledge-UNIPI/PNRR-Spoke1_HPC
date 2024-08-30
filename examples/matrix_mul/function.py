import numpy as np
import json

def handler(params, context):
    try:
        #matrix_a = np.array(params['mat_a'])
        #matrix_b = np.array(params['mat_b'])
        
        size = 5000
        matrix_a = np.random.rand(size, size)
        matrix_b = np.random.rand(size, size)

        result = np.dot(matrix_a, matrix_b)
        mean_value = np.mean(result)

        return json.dumps({"mean_value": mean_value})
    except Exception as e:
        print(e)
        result = {"Error": str(e)}

    return result