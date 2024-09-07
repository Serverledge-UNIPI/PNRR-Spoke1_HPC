def large_integer_factorization(n):
    def smallest_factor(n):
        if n % 2 == 0:
            return 2
        for i in range(3, int(n**0.5) + 1, 2):
            if n % i == 0:
                return i
        return n

    factors = []
    while n > 1:
        factor = smallest_factor(n)
        factors.append(factor)
        n //= factor

    return factors

def handler(params, context):
    try:
        n = int(params["n"])
        result = large_integer_factorization(n)
        return {"Prime numbers": result}
    except:
        return {}
