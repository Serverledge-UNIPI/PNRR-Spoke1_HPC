#ifndef PY_SOLVER_H
#define PY_SOLVER_H

#include <Python.h>
#include <stdio.h>

void initializePython() {
	if (!Py_IsInitialized()) {
		Py_Initialize();
        PyRun_SimpleString("import sys; sys.path.insert(0, './internal/solver/')");
	}
}

void finalizePython() {
	if (Py_IsInitialized()) {
		Py_Finalize();
	}
}

int* allocateMemory(int size) {
    int* array = (int*)malloc(size * sizeof(int));
    return array;
}

void freeMemory(int* array) {
    free(array);
}

const char* startSolver(int numberOfNodes, int numberOfFunctions, int* nodeMemory, int* nodeCapacity, int* maximumCapacity, int* nodeIpc, int* nodePowerConsumption, int* functionMemory, int* functionWorkload, int* functionDeadline, int* functionInvocations) {
    PyObject *pName, *pModule, *pFunc;
    PyObject *pArgs, *pValue;
    const char* result;

    pName = PyUnicode_DecodeFSDefault("solver_wrapper");
    pModule = PyImport_Import(pName);
    Py_DECREF(pName);

    if (pModule != NULL) {
        pFunc = PyObject_GetAttrString(pModule, "start_solver");
        if (pFunc && PyCallable_Check(pFunc)) {
            pArgs = PyTuple_New(11); 

            // Set the arguments in the tuple
            PyTuple_SetItem(pArgs, 0, PyLong_FromLong(numberOfNodes));
            PyTuple_SetItem(pArgs, 1, PyLong_FromLong(numberOfFunctions));
            PyTuple_SetItem(pArgs, 2, PyList_New(numberOfNodes));
            PyTuple_SetItem(pArgs, 3, PyList_New(numberOfNodes));
            PyTuple_SetItem(pArgs, 4, PyList_New(numberOfNodes));
            PyTuple_SetItem(pArgs, 5, PyList_New(numberOfNodes));
            PyTuple_SetItem(pArgs, 6, PyList_New(numberOfNodes));
            PyTuple_SetItem(pArgs, 7, PyList_New(numberOfFunctions));
            PyTuple_SetItem(pArgs, 8, PyList_New(numberOfFunctions));
            PyTuple_SetItem(pArgs, 9, PyList_New(numberOfFunctions));
            PyTuple_SetItem(pArgs, 10, PyList_New(numberOfFunctions));

            // Populate nodeMemory, nodeCapacity, nodeIpc, nodePowerConsumption into PyList objects
            for (int i = 0; i < numberOfNodes; i++) {
                PyList_SetItem(PyTuple_GetItem(pArgs, 2), i, PyLong_FromLong(nodeMemory[i]));
                PyList_SetItem(PyTuple_GetItem(pArgs, 3), i, PyLong_FromLong(nodeCapacity[i]));
                PyList_SetItem(PyTuple_GetItem(pArgs, 4), i, PyLong_FromLong(maximumCapacity[i]));
                PyList_SetItem(PyTuple_GetItem(pArgs, 5), i, PyLong_FromLong(nodeIpc[i]));
                PyList_SetItem(PyTuple_GetItem(pArgs, 6), i, PyLong_FromLong(nodePowerConsumption[i]));
            }

            // Populate functionMemory, functionWorkload, functionDeadline, functionInvocations into PyList objects
            for (int j = 0; j < numberOfFunctions; j++) {
                PyList_SetItem(PyTuple_GetItem(pArgs, 7), j, PyLong_FromLong(functionMemory[j]));
                PyList_SetItem(PyTuple_GetItem(pArgs, 8), j, PyLong_FromLong(functionWorkload[j]));
                PyList_SetItem(PyTuple_GetItem(pArgs, 9), j, PyLong_FromLong(functionDeadline[j]));
                PyList_SetItem(PyTuple_GetItem(pArgs, 10), j, PyLong_FromLong(functionInvocations[j]));
            }

            // Call the Python function
            pValue = PyObject_CallObject(pFunc, pArgs);
            Py_DECREF(pArgs);

            if (pValue != NULL) {
                if (!PyUnicode_Check(pValue)) {
                    fprintf(stderr, "PyObject is not a string\n");
                    return NULL;
                } else {
                    result = PyUnicode_AsUTF8(pValue);
                }

                Py_DECREF(pValue);         
            } else {
                PyErr_Print();
                fprintf(stderr, "Function call failed\n");
                return NULL;
            }
        } else {
            if (PyErr_Occurred())
                PyErr_Print();
            Py_XDECREF(pFunc);
            Py_DECREF(pModule);
            return NULL;
        }
    } else {
        PyErr_Print();
        return NULL;
    }
    return result;
}

#endif // PY_SOLVER_H