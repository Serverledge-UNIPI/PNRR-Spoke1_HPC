import numpy as np

def handler(params, context):
    try:
        matrix_a = np.array(params['matrix_a'])
        matrix_b = np.array(params['matrix_b'])

        result = np.dot(matrix_a, matrix_b)
        mean_value = np.mean(result)

        return {"Mean_value": mean_value}
    except:
        return {}